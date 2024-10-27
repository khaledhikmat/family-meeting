package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	firebase "firebase.google.com/go/v4"
	"github.com/mdobak/go-xerrors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/khaledhikmat/family-meeting/service/lgr"
	"github.com/khaledhikmat/family-meeting/utils"
)

const (
	projectID       = "family-meeting-aa853"
	broadcastsTopic = "broadcasts"
)

var (
	meter = otel.Meter("family.meeting.monitor")

	publishDuration metric.Int64Histogram
)

func init() {
	var err error
	publishDuration, err = meter.Int64Histogram(
		"family.meeting.monitor.publish.duration",
		metric.WithDescription("The distribution of publish durations"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		lgr.Logger.Error(
			"creating histogram",
			slog.Any("error", xerrors.New(err.Error())),
		)
	}
}

func Processor(canxCtx context.Context,
	_ *firebase.App,
	db *firestore.Client,
	errorStream chan error) error {

	lgr.Logger.Info("monitor proc started")

	client, err := pubsub.NewClient(canxCtx, projectID)
	if err != nil {
		lgr.Logger.Error(
			"creating pubsub client",
			slog.Any("error", xerrors.New(err.Error())),
		)
		return err
	}
	defer client.Close()

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
			slog.Any("error", xerrors.New(err.Error())),
		)
		return err
	}

	// Monitor for broadcaster requests
	broadcastReqStream := utils.MonitorRequests(canxCtx, nil, nil, errorStream, db, "broadcaster", "")

	for {
		select {
		case <-canxCtx.Done():
			lgr.Logger.Info(
				"monitor proc context cancelled",
			)
			return nil
		case broadcastReqDoc := <-broadcastReqStream:
			// Publish a message (as broadcast_request ID) to kick start a broadcaster
			now := time.Now()
			result := t.Publish(canxCtx, &pubsub.Message{
				Data: []byte(broadcastReqDoc.Ref.ID),
			})
			id, err := result.Get(canxCtx)
			if err != nil {
				errorStream <- fmt.Errorf("error publishing message: %v", err)
			}
			lgr.Logger.Info(
				"monitor proc published message",
				slog.String("broadcast_id", id),
			)
			publishDuration.Record(canxCtx, time.Since(now).Milliseconds())
		}
	}
}
