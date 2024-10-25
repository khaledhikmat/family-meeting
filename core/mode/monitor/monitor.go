package monitor

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	firebase "firebase.google.com/go/v4"

	"github.com/khaledhikmat/family-meeting/utils"
)

const (
	projectID       = "family-meeting-aa853"
	broadcastsTopic = "broadcasts"
)

func Processor(canxCtx context.Context,
	_ *firebase.App,
	db *firestore.Client,
	errorStream chan error) error {

	fmt.Println("monitor proc started")

	client, err := pubsub.NewClient(canxCtx, projectID)
	if err != nil {
		return fmt.Errorf("error creating pubsub client: %v", err)
	}
	defer client.Close()

	t := client.Topic(broadcastsTopic)
	defer t.Stop()

	ok, err := t.Exists(canxCtx)
	if err != nil {
		return fmt.Errorf("error checking topic %s existence: %v", broadcastsTopic, err)
	}

	if !ok {
		return fmt.Errorf("topic %s does not exist", broadcastsTopic)
	}

	// Monitor for broadcaster requests
	broadcastReqStream := utils.MonitorRequests(canxCtx, nil, nil, errorStream, db, "broadcaster", "")

	for {
		select {
		case <-canxCtx.Done():
			fmt.Printf("monitor proc context cancelled: %v\n", canxCtx.Err())
			return nil
		case broadcastReqDoc := <-broadcastReqStream:
			// Publish a message (as broadcast_request ID) to kick start a broadcaster
			result := t.Publish(canxCtx, &pubsub.Message{
				Data: []byte(broadcastReqDoc.Ref.ID),
			})
			id, err := result.Get(canxCtx)
			if err != nil {
				errorStream <- fmt.Errorf("error publishing message: %v", err)
			}
			fmt.Printf("monitor proc published message ID: %v\n", id)
		}
	}
}
