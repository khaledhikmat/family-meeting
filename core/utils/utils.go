package utils

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/pion/webrtc/v4"
)

type Request struct {
	ID        string `json:"id"`
	Parent    string `json:"parent"`
	Requestor string `json:"requestor"`
	Kind      string `json:"kind"`
	Offer     string `json:"offer"`
	Answer    string `json:"answer"`
	Abort     bool   `json:"abort"`
}

func MonitorRequests(canxCtx context.Context,
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

func WaitForOffer(canxCtx context.Context,
	requestCanxCtx context.Context,
	errorStream chan error,
	_ *firestore.Client,
	reqDoc *firestore.DocumentRef) webrtc.SessionDescription {
	// Wait until an offer is made by a requestor (kind)
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
			request := Request{}
			err = snap.DataTo(&request)
			if err != nil {
				errorStream <- fmt.Errorf("waitForOffer error getting request data: %v", err)
				continue
			}
			if request.Offer == "" {
				continue
			}
			offer := webrtc.SessionDescription{}
			Decode(request.Offer, &offer)
			return offer
		}
	}
}

// JSON encode + base64 a SessionDescription
func Encode(obj *webrtc.SessionDescription) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

// Decode a base64 and unmarshal JSON into a SessionDescription
func Decode(in string, obj *webrtc.SessionDescription) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}

	if err = json.Unmarshal(b, obj); err != nil {
		panic(err)
	}
}
