package broadcast

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	firebase "firebase.google.com/go/v4"
	"github.com/mdobak/go-xerrors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/khaledhikmat/family-meeting/service/lgr"
	"github.com/khaledhikmat/family-meeting/utils"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v4"
)

const (
	projectID            = "family-meeting-aa853"
	abortWatcherInterval = 5 * time.Second
	waitOnTrackTimeout   = 30 * time.Second
	broadcastsTopic      = "broadcasts"
	broadcastsSub        = "broadcasts-sub"
)

var (
	meter = otel.Meter("family.meeting.broadcast")

	receiveDuration metric.Int64Histogram
)

func init() {
	var err error
	receiveDuration, err = meter.Int64Histogram(
		"family.meeting.broadcast.receive.duration",
		metric.WithDescription("The distribution of receive durations"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		lgr.Logger.Error(
			"creating histogram",
			slog.Any("error", xerrors.New(err.Error())),
		)
	}
}

// Must match the one configured in client JavaScript
var peerConnectionConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

type track struct {
	Ctx   context.Context
	CtxFn context.CancelFunc
	Track *webrtc.TrackLocalStaticRTP
}

func Processor(canxCtx context.Context,
	_ *firebase.App,
	db *firestore.Client,
	errorStream chan error) error {

	lgr.Logger.Info("broadcast proc started")

	client, err := pubsub.NewClient(canxCtx, projectID)
	if err != nil {
		lgr.Logger.Error(
			"creating pubsub client",
			slog.Any("error", xerrors.New(err.Error())),
		)
		return err
	}
	defer client.Close()

	// Make sure topic exists
	t := client.Topic(broadcastsTopic)
	defer t.Stop()

	ok, err := t.Exists(canxCtx)
	if err != nil {
		lgr.Logger.Error(
			"checking topic",
			slog.String("topic", broadcastsTopic),
			slog.Any("error", xerrors.New(err.Error())),
		)
		return err
	}

	if !ok {
		lgr.Logger.Error(
			"checking topic",
			slog.String("topic", broadcastsTopic),
			slog.Any("error", xerrors.New("topic does not exist")),
		)
		return err
	}

	// Make sure topic subscription exists
	sub := client.Subscription(broadcastsSub)

	ok, err = sub.Exists(canxCtx)
	if err != nil {
		lgr.Logger.Error(
			"checking topic sub",
			slog.String("topic", broadcastsTopic),
			slog.String("sub", broadcastsSub),
			slog.Any("error", xerrors.New("topic subscription does not exist")),
		)
		return err
	}

	if !ok {
		lgr.Logger.Error(
			"checking topic sub",
			slog.String("topic", broadcastsTopic),
			slog.String("sub", broadcastsSub),
			slog.Any("error", xerrors.New("topic sub does not exist")),
		)
		return err
	}

	// Consume events from the topic
	// Receive blocks until a message is received or the context is cancelled
	// There is more control: https://cloud.google.com/pubsub/docs/samples/pubsub-subscriber-concurrency-control?hl=en
	err = sub.Receive(canxCtx, func(_ context.Context, msg *pubsub.Message) {
		now := time.Now()
		defer msg.Ack()
		lgr.Logger.Info("broadcast proc received message",
			slog.String("msg", string(msg.Data)),
		)

		// Consume from a queue to start broadcasters
		go startBroadcaster(canxCtx,
			errorStream,
			db,
			string(msg.Data))

		receiveDuration.Record(canxCtx, time.Since(now).Milliseconds())
	})
	if err != nil {
		lgr.Logger.Error(
			"broadcast proc receiving message",
			slog.String("topic", broadcastsTopic),
			slog.String("sub", broadcastsSub),
			slog.Any("error", xerrors.New(err.Error())),
		)

		return err
	}

	for range canxCtx.Done() {
		lgr.Logger.Info("broadcast proc context cancelled")
		return nil
	}

	return nil
}

func startBroadcaster(canxCtx context.Context,
	errorStream chan error,
	db *firestore.Client,
	broadcastID string) {
	reqRef := db.Collection("broadcast_requests")
	broadcastReq := reqRef.Doc(broadcastID)
	if broadcastReq == nil {
		errorStream <- fmt.Errorf("startBroadcaster broadcastDoc is nil")
		return
	}

	requestCanxCtx, requestCanxFn := context.WithCancel(canxCtx)
	defer requestCanxFn()

	// Wait until an offer is created by the broadcaster
	offer := utils.WaitForOffer(canxCtx, requestCanxCtx, errorStream, db, broadcastReq)

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

	localTrackStream := make(chan track)
	defer close(localTrackStream)

	// Set a handler for when a new remote track starts, this just distributes all our packets
	// to connected peers
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		// WARNING: This should happen only once and arrives on its own goroutine
		lgr.Logger.Info("startBroadcaster onTrack should happen only once")
		// TODO: For some reason, this happens more than once, we need to investigate why
		// I am allowing this to happen
		lgr.Logger.Info("startBroadcaster peerConnection.OnTrack from the remote broadcaster")
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
	_, err = broadcastReq.Update(canxCtx, []firestore.Update{{
		Path: "answer", Value: utils.Encode(peerConnection.LocalDescription()),
	}})
	if err != nil {
		errorStream <- fmt.Errorf("startBroadcaster broadcastReq.Update error: %v", err)
		return
	}

	// Wait to receive cancellation or abort
	go func() {
		ticker := time.NewTicker(abortWatcherInterval)
		defer ticker.Stop()

		for {
			select {
			case <-canxCtx.Done():
				lgr.Logger.Info("abortWatcher context cancelled")
				return
			case <-requestCanxCtx.Done():
				lgr.Logger.Info("abortWatcher request context cancelled")
				return
			case <-ticker.C:
				reqRef := db.Collection("broadcast_requests")
				broadcastDoc := reqRef.Doc(broadcastReq.ID)
				docSnap, err := broadcastDoc.Get(canxCtx)
				if err != nil {
					errorStream <- fmt.Errorf("abortWatcherfailed to get document: %v", err)
					continue
				}
				var request utils.Request
				err = docSnap.DataTo(&request)
				if err != nil {
					errorStream <- fmt.Errorf("abortWatcher failed to decode document: %v", err)
					continue
				}

				if request.Abort {
					lgr.Logger.Info("abortWatcher aborting the broadcast",
						slog.String("broadcast", broadcastReq.ID),
					)
					requestCanxFn()
					return
				}
			}
		}
	}()

	var localTrack track
	var canxFn context.CancelFunc

	// Timer to wait for a remote track to arrive
	// Unfortunately, this means that the broadcaster will wait for a track to arrive within this time
	// This also means that the broadcaster will not be able to process participant requests until this time expires
	// The reason we don't proceed once a track arrives is because it is possible (based on experimentation)
	// that the onTrack event is called multiple times
	timer := time.NewTimer(waitOnTrackTimeout)
	defer timer.Stop()

	// Wait to receive cancellation, local track or timeout
	for {
		select {
		case <-canxCtx.Done():
			lgr.Logger.Info("startBroadcaster context cancelled")
			return
		case <-requestCanxCtx.Done():
			lgr.Logger.Info("startBroadcaster request context cancelled")
			return
		case <-timer.C:
			// Timer expired, resume with waiting on participant requests
			lgr.Logger.Info(
				"startBroadcaster timeout to receive a remote track occurred. Resume.",
			)
			goto resume
		case localTrack = <-localTrackStream:
			lgr.Logger.Info("startBroadcaster received a remote track. Now I can accept participants")
			if canxFn != nil {
				lgr.Logger.Info("startBroadcaster received a remote track. Cancelling previous track context")
				canxFn()
			}
			canxFn = localTrack.CtxFn
		}
	}

resume:
	// if the local track is still nil, the broadcaster will exit immediately
	if localTrack.Track == nil {
		errorStream <- fmt.Errorf("startBroadcaster did not receive a track in %v. Exiting", waitOnTrackTimeout)
		return
	}

	// Monitor participant requests
	participantReqStream := utils.MonitorRequests(canxCtx, requestCanxCtx, requestCanxFn, errorStream, db, "participant", broadcastReq.ID)

	// Wait to receive participant requests
	for {
		select {
		case <-canxCtx.Done():
			lgr.Logger.Info("startBroadcaster context cancelled")
			return
		case <-requestCanxCtx.Done():
			lgr.Logger.Info("startBroadcaster request context cancelled")
			return
		case participantReqDoc := <-participantReqStream:
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
	lgr.Logger.Info("onRemoteTrack should happen only once")
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

	// Form a track object to stream to the localTrackStream
	localTrackStream <- track{
		Ctx:   myCanxCtx,
		CtxFn: myCanxFn,
		Track: localTrack,
	}

	// Stream the incoming RTP packets to the local track
	// Buffer the packets to avoid blocking the RTP stream
	rtpStream := make(chan []byte, 100)
	defer close(rtpStream)

	// Create a goroutine to stream RTP packets to the local track
	go func() {
		if os.Getenv("EXPERIMENT_RTP_SEP_RW") != "true" {
			return
		}

		for {
			select {
			case <-myCanxCtx.Done():
				lgr.Logger.Info("rtpStreamer my context cancelled",
					slog.Any("track", localTrack),
				)
				return
			case <-canxCtx.Done():
				lgr.Logger.Info("rtpStreamer context cancelled",
					slog.Any("track", localTrack),
				)
				return
			case <-requestCanxCtx.Done():
				lgr.Logger.Info("rtpStreamer request context cancelled",
					slog.Any("track", localTrack),
				)
				return
			case data := <-rtpStream:
				if _, err := localTrack.Write(data); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					errorStream <- fmt.Errorf("rtpStreamer %p localTrack.Write error: %v", localTrack, err)
					continue
				}
			}
		}
	}()

	// Read RTP data and stream it to be broadcasted to WebRTC peers
	rtpBuf := make([]byte, 1400)
	for {
		select {
		case <-myCanxCtx.Done():
			lgr.Logger.Info("onRemoteTrack my context cancelled",
				slog.Any("track", localTrack),
			)
			return
		case <-canxCtx.Done():
			lgr.Logger.Info("onRemoteTrack context cancelled",
				slog.Any("track", localTrack),
			)
			return
		case <-requestCanxCtx.Done():
			lgr.Logger.Info("onRemoteTrack request context cancelled",
				slog.Any("track", localTrack),
			)
			return
		default:
			i, _, readErr := remoteTrack.Read(rtpBuf)
			if readErr != nil {
				//errorStream <- fmt.Errorf("onRemoteTrack remoteTrack.Read error: %v", readErr)
				continue
			}

			// EXPERIMENTATION:
			// Separate reading RTP packets from writing RTP packets to local track
			// Hopefully this will solve pixalation issues at the peers
			if os.Getenv("EXPERIMENT_RTP_SEP_RW") == "true" {
				rtpStream <- rtpBuf[:i]
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
	participantOffer := utils.WaitForOffer(canxCtx, requestCanxCtx, errorStream, db, participantReq)
	lgr.Logger.Info("startParticipant received offer from a participant")
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
				lgr.Logger.Info("startParticipant RTCP context cancelled")
				return
			case <-requestCanxCtx.Done():
				lgr.Logger.Info("startParticipant RTCP request context cancelled")
				return
			default:
				if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
					errorStream <- fmt.Errorf("startParticipant RTCP rtpSender.Read error: %v", err)
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
	_, err = participantReq.Update(canxCtx, []firestore.Update{
		{
			Path:  "answer",
			Value: utils.Encode(peerConnection.LocalDescription()),
		},
	})
	if err != nil {
		errorStream <- fmt.Errorf("startParticipant participantReq.Update error: %v", err)
		return
	}

	for {
		select {
		case <-canxCtx.Done():
			lgr.Logger.Info("startParticipant context cancelled")
			return
		case <-requestCanxCtx.Done():
			lgr.Logger.Info("startParticipant request context cancelled")
			return
		}
	}
}
