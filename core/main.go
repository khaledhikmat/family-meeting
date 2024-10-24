package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v4"
)

const (
	abortWatcherInterval = 5 * time.Second
)

// Must match the one configured in client JavaScript
var peerConnectionConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

type request struct {
	ID        string `json:"id"`
	Parent    string `json:"parent"`
	Requestor string `json:"requestor"`
	Kind      string `json:"kind"`
	Offer     string `json:"offer"`
	Answer    string `json:"answer"`
	Abort     bool   `json:"abort"`
}

type track struct {
	Ctx   context.Context
	CtxFn context.CancelFunc
	Track *webrtc.TrackLocalStaticRTP
}

func main() {
	rootCtx := context.Background()
	canxCtx, canxFn := context.WithCancel(rootCtx)

	// Hook up a signal handler to cancel the context
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v, cancelling context...\n", sig)
		canxFn()
	}()
	app, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		log.Fatalf("error initializing firebase app: %v\n", err)
		return
	}

	db, err := app.Firestore(rootCtx)
	if err != nil {
		log.Fatalf("error acquiring firestore client: %v\n", err)
		return
	}

	// Create an error stream
	errorStream := make(chan error)
	defer close(errorStream)

	// Create a centralized error processor
	go streamError(canxCtx, errorStream)

	// Monitor for broadcaster requests
	broadcastReqStream := monitorRequests(canxCtx, nil, nil, errorStream, db, "broadcaster", "")

	for {
		select {
		case <-canxCtx.Done():
			fmt.Printf("main context cancelled: %v\n", canxCtx.Err())
			//canxFn()
			// Wait 4 seconds for all the go routines to exit
			time.Sleep(4 * time.Second)
			return
		case broadcastReqDoc := <-broadcastReqStream:
			go startBroadcaster(canxCtx, errorStream, db, broadcastReqDoc.Ref)
		}
	}
}

func streamError(canxCtx context.Context, errorStream chan error) {
	for {
		select {
		case <-canxCtx.Done():
			fmt.Printf("streamError context cancelled: %v\n", canxCtx.Err())
			return
		case e := <-errorStream:
			fmt.Printf("streamError processor processed this error: %v\n", e)
		}
	}
}

func monitorRequests(canxCtx context.Context,
	requestCanxCtx context.Context,
	_ context.CancelFunc,
	errorStream chan error,
	db *firestore.Client,
	kind string,
	parent string) chan *firestore.DocumentSnapshot {
	requestsChan := make(chan *firestore.DocumentSnapshot)

	go func() {
		defer close(requestsChan)

		reqRef := db.Collection("broadcast_requests").
			Where("kind", "==", kind).
			Where("parent", "==", parent).
			Where("offer", "!=", "").
			Where("answer", "==", "").
			Where("abort", "==", false)
		//q := reqRef.OrderBy("createdAt", firestore.Desc).Limit(3)

		iter := reqRef.Snapshots(canxCtx)
		defer iter.Stop()

		for {
			if canxCtx.Err() != nil {
				errorStream <- fmt.Errorf("monitorRequests context cancelled: %v", canxCtx.Err())
				return
			}

			if requestCanxCtx != nil && requestCanxCtx.Err() != nil {
				errorStream <- fmt.Errorf("monitorRequests parent context cancelled: %v", requestCanxCtx.Err())
				return
			}

			// TODO: This is a blocking atomic operation!!!!
			// While waiting, we are not checking for context cancellation
			snap, err := iter.Next()
			if err != nil {
				errorStream <- fmt.Errorf("monitorRequests error getting snapshot: %v", err)
				continue
			}
			for _, change := range snap.Changes {
				if change.Kind == firestore.DocumentAdded {
					requestsChan <- change.Doc
				}
			}
		}
	}()

	return requestsChan
}

func waitForOffer(canxCtx context.Context,
	requestCanxCtx context.Context,
	errorStream chan error,
	_ *firestore.Client,
	reqDoc *firestore.DocumentRef) webrtc.SessionDescription {
	// Wait until an offer is made by a requestor (kind)
	// TODO: timeout after 5 minutes
	iter := reqDoc.Snapshots(canxCtx)
	defer iter.Stop()

	for {
		if canxCtx.Err() != nil {
			errorStream <- fmt.Errorf("waitForOffer context cancelled: %v", canxCtx.Err())
			return webrtc.SessionDescription{}
		}

		if requestCanxCtx != nil && requestCanxCtx.Err() != nil {
			errorStream <- fmt.Errorf("waitForOffer parent context cancelled: %v", requestCanxCtx.Err())
			return webrtc.SessionDescription{}
		}

		// TODO: This is a blocking atomic operation!!!!
		// While waiting, we are not checking for context cancellation
		snap, err := iter.Next()
		if err != nil {
			errorStream <- fmt.Errorf("waitForOffer error getting snapshot: %v", err)
			continue
		}
		if snap.Exists() {
			request := request{}
			err = snap.DataTo(&request)
			if err != nil {
				errorStream <- fmt.Errorf("waitForOffer error getting request data: %v", err)
				continue
			}
			if request.Offer == "" {
				continue
			}
			offer := webrtc.SessionDescription{}
			decode(request.Offer, &offer)
			return offer
		}
	}
}

func startBroadcaster(canxCtx context.Context,
	errorStream chan error,
	db *firestore.Client,
	broadcastReq *firestore.DocumentRef) {
	requestCanxCtx, requestCanxFn := context.WithCancel(canxCtx)
	defer requestCanxFn()

	// Wait until an offer is created by the broadcaster
	offer := waitForOffer(canxCtx, requestCanxCtx, errorStream, db, broadcastReq)

	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		errorStream <- fmt.Errorf("startBroadcaster RegisterDefaultCodecs error: %v", err)
		return
	}

	// Create a InterceptorRegistry. This is the user configurable RTP/RTCP Pipeline.
	// This provides NACKs, RTCP Reports and other features. If you use `webrtc.NewPeerConnection`
	// this is enabled by default. If you are manually managing You MUST create a InterceptorRegistry
	// for each PeerConnection.
	i := &interceptor.Registry{}

	// Use the default set of Interceptors
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		errorStream <- fmt.Errorf("startBroadcaster RegisterDefaultInterceptors error: %v", err)
		return
	}

	// Register a intervalpli factory
	// This interceptor sends a PLI every 3 seconds. A PLI causes a video keyframe to be generated by the sender.
	// This makes our video seekable and more error resilent, but at a cost of lower picture quality and higher bitrates
	// A real world application should process incoming RTCP packets from viewers and forward them to senders
	intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		errorStream <- fmt.Errorf("startBroadcaster NewReceiverInterceptor error: %v", err)
		return
	}
	i.Add(intervalPliFactory)

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i)).NewPeerConnection(peerConnectionConfig)
	if err != nil {
		errorStream <- fmt.Errorf("startBroadcaster NewAPI error: %v", err)
		return
	}
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			errorStream <- fmt.Errorf("startBroadcaster cannot close peerConnection: %v", cErr)
		}
	}()

	// Allow us to receive 1 video track
	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
		errorStream <- fmt.Errorf("startBroadcaster AddTransceiverFromKind error: %v", err)
		return
	}

	// localTrackStream := make(chan *webrtc.TrackLocalStaticRTP)
	localTrackStream := make(chan track)
	defer close(localTrackStream)

	// Set a handler for when a new remote track starts, this just distributes all our packets
	// to connected peers
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		// WARNING: This should happen only once and arrives on its own goroutine
		fmt.Println("startBroadcaster onTrack should happen only once")
		// TODO: For some reason, this happens more than once, we need to investigate why
		// I am allowing this to happen
		fmt.Println("startBroadcaster peerConnection.OnTrack from the remote broadcaster")
		go onRemoteTrack(canxCtx, requestCanxCtx, errorStream, localTrackStream, remoteTrack, receiver)
	})

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		errorStream <- fmt.Errorf("startBroadcaster SetRemoteDescription error: %v", err)
		return
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		errorStream <- fmt.Errorf("startBroadcaster CreateAnswer error: %v", err)
		return
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		errorStream <- fmt.Errorf("startBroadcaster SetLocalDescription error: %v", err)
		return
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	// Update the answer in the broacast request
	_, err = broadcastReq.Update(canxCtx, []firestore.Update{{Path: "answer", Value: encode(peerConnection.LocalDescription())}})
	if err != nil {
		errorStream <- fmt.Errorf("startBroadcaster broadcastReq.Update error: %v", err)
		return
	}

	// TODO: Wait until we connect with the remote broadcaster
	// This is an atomic operation, we are not checking for context cancellation
	//localTrack := <-localTrackStream
	//fmt.Printf("startBroadcaster received a remote track\n")
	var localTrack track //*webrtc.TrackLocalStaticRTP

	participantReqStream := monitorRequests(canxCtx, requestCanxCtx, requestCanxFn, errorStream, db, "participant", broadcastReq.ID)
	fmt.Printf("startBroadcaster done setting up participants monitor\n")

	// Abort watcher
	go func() {
		ticker := time.NewTicker(abortWatcherInterval)
		defer ticker.Stop()

		for {
			select {
			case <-canxCtx.Done():
				fmt.Printf("abortWatcher context cancelled: %v\n", canxCtx.Err())
				return
			case <-requestCanxCtx.Done():
				fmt.Printf("abortWatcher request context cancelled: %v\n", canxCtx.Err())
				return
			case <-ticker.C:
				reqRef := db.Collection("broadcast_requests")
				broadcastDoc := reqRef.Doc(broadcastReq.ID)
				docSnap, err := broadcastDoc.Get(canxCtx)
				if err != nil {
					errorStream <- fmt.Errorf("abortWatcherfailed to get document: %v", err)
					continue
				}
				var request request
				err = docSnap.DataTo(&request)
				if err != nil {
					errorStream <- fmt.Errorf("abortWatcher failed to decode document: %v", err)
					continue
				}

				if request.Abort {
					fmt.Printf("abortWatcher aborting the broadcast %s\n", broadcastReq.ID)
					requestCanxFn()
					return
				}
			}
		}
	}()

	var canxFn context.CancelFunc

	// Wait to receive participant requests
	for {
		select {
		case <-canxCtx.Done():
			fmt.Printf("startBroadcaster context cancelled: %v\n", canxCtx.Err())
			return
		case <-requestCanxCtx.Done():
			fmt.Printf("startBroadcaster request context cancelled: %v\n", requestCanxCtx.Err())
			return
		case localTrack = <-localTrackStream:
			fmt.Printf("startBroadcaster received a remote track. Now I can accept participants\n")
			if canxFn != nil {
				fmt.Printf("startBroadcaster received a remote track. Cancelling previous track context\n")
				canxFn()
			}
			canxFn = localTrack.CtxFn
		case participantReqDoc := <-participantReqStream:
			// WARNING: if the localTrack is nil, startParticipant will exit immediately
			go startParticipant(canxCtx, requestCanxCtx, errorStream, db, participantReqDoc.Ref, localTrack.Track)
		}
	}
}

func onRemoteTrack(canxCtx context.Context,
	requestCanxCtx context.Context,
	errorStream chan error,
	localTrackStream chan track,
	remoteTrack *webrtc.TrackRemote,
	_ *webrtc.RTPReceiver) {
	fmt.Println("onRemoteTrack should happen only once")
	// But I have noticed that it happens more than once
	// So I am creating a child context for each track
	// this way I can cancel the track context when a new one arrives

	// Get a child context for this track
	myCanxCtx, myCanxFn := context.WithCancel(canxCtx)
	defer myCanxFn()

	// Create a local track, all our SFU clients will be fed via this track
	localTrack, newTrackErr := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, "video", "family-meeting")
	if newTrackErr != nil {
		errorStream <- fmt.Errorf("onRemoteTrack NewTrackLocalStaticRTP error: %v", newTrackErr)
		return
	}

	// WARNING: This is a blocking operation in case onRemoteTrack is called multiple times
	localTrackStream <- track{
		Ctx:   myCanxCtx,
		CtxFn: myCanxFn,
		Track: localTrack,
	}

	rtpBuf := make([]byte, 1400)
	for {
		select {
		case <-myCanxCtx.Done():
			fmt.Printf("onRemoteTrack %p my context cancelled: %v\n", localTrack, myCanxCtx.Err())
			return
		case <-canxCtx.Done():
			fmt.Printf("onRemoteTrack %p context cancelled: %v\n", localTrack, canxCtx.Err())
			return
		case <-requestCanxCtx.Done():
			fmt.Printf("onRemoteTrack %p request context cancelled: %v\n", localTrack, requestCanxCtx.Err())
			return
		default:
			i, _, readErr := remoteTrack.Read(rtpBuf)
			if readErr != nil {
				//errorStream <- fmt.Errorf("onRemoteTrack remoteTrack.Read error: %v", readErr)
				continue
			}

			// ErrClosedPipe means we don't have any subscribers, this is ok if no peers have connected yet
			if _, err := localTrack.Write(rtpBuf[:i]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
				errorStream <- fmt.Errorf("onRemoteTrack %p localTrack.Write error: %v", localTrack, err)
				continue
			}
		}
	}
}

func startParticipant(canxCtx context.Context,
	requestCanxCtx context.Context,
	errorStream chan error,
	db *firestore.Client,
	participantReq *firestore.DocumentRef,
	localTrack *webrtc.TrackLocalStaticRTP) {
	participantOffer := waitForOffer(canxCtx, requestCanxCtx, errorStream, db, participantReq)
	fmt.Printf("startParticipant received offer from a participant\n")
	if localTrack == nil {
		errorStream <- fmt.Errorf("startParticipant localTrack is nil. Exiting")
		return
	}

	// Create a new PeerConnection
	peerConnection, err := webrtc.NewPeerConnection(peerConnectionConfig)
	if err != nil {
		errorStream <- fmt.Errorf("startParticipant NewPeerConnection error: %v", err)
		return
	}

	rtpSender, err := peerConnection.AddTrack(localTrack)
	if err != nil {
		errorStream <- fmt.Errorf("startParticipant AddTrack error: %v", err)
		return
	}

	// Read incoming RTCP packets
	// Before these packets are returned they are processed by interceptors. For things
	// like NACK this needs to be called.
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			select {
			case <-canxCtx.Done():
				fmt.Printf("startParticipant RTCP context cancelled: %v\n", canxCtx.Err())
				return
			case <-requestCanxCtx.Done():
				fmt.Printf("startParticipant RTCP request context cancelled: %v\n", requestCanxCtx.Err())
				return
			default:
				if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
					errorStream <- fmt.Errorf("startParticipant rtpSender.Read error: %v", err)
					return
				}
			}
		}
	}()

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(participantOffer)
	if err != nil {
		errorStream <- fmt.Errorf("startParticipant SetRemoteDescription error: %v", err)
		return
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		errorStream <- fmt.Errorf("startParticipant CreateAnswer error: %v", err)
		return
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		errorStream <- fmt.Errorf("startParticipant SetLocalDescription error: %v", err)
		return
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	// Update the answer in the participant request
	_, err = participantReq.Update(canxCtx, []firestore.Update{{Path: "answer", Value: encode(peerConnection.LocalDescription())}})
	if err != nil {
		errorStream <- fmt.Errorf("startParticipant participantReq.Update error: %v", err)
		return
	}

	for {
		select {
		case <-canxCtx.Done():
			fmt.Printf("startParticipant context cancelled: %v\n", canxCtx.Err())
			return
		case <-requestCanxCtx.Done():
			fmt.Printf("startParticipant request context cancelled: %v\n", requestCanxCtx.Err())
			return
		}
	}
}

// JSON encode + base64 a SessionDescription
func encode(obj *webrtc.SessionDescription) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

// Decode a base64 and unmarshal JSON into a SessionDescription
func decode(in string, obj *webrtc.SessionDescription) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}

	if err = json.Unmarshal(b, obj); err != nil {
		panic(err)
	}
}
