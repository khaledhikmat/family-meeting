package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	firebase "firebase.google.com/go/v4"
	"github.com/mdobak/go-xerrors"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	noopmeter "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"github.com/khaledhikmat/family-meeting/server"
	"github.com/khaledhikmat/family-meeting/service/lgr"

	"github.com/khaledhikmat/family-meeting/mode"
	"github.com/khaledhikmat/family-meeting/mode/broadcast"
	"github.com/khaledhikmat/family-meeting/mode/monitor"
)

const (
	waitOnShutdown = 4 * time.Second
)

var modeProcs = map[string]mode.Processor{
	"monitor":   monitor.Processor,
	"broadcast": broadcast.Processor,
}

func main() {
	rootCtx := context.Background()
	canxCtx, canxFn := context.WithCancel(rootCtx)

	// Hook up a signal handler to cancel the context
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		lgr.Logger.Info(
			"received kill signal",
			slog.Any("signal", sig),
		)
		canxFn()
	}()

	// Setup OpenTelemetry
	shutdown, err := setupOpenTelemetry(rootCtx)
	if err != nil {
		lgr.Logger.Error(
			"setting up OpenTelemetry",
			slog.Any("error", xerrors.New(err.Error())),
		)
		return
	}
	defer func() {
		err := shutdown(rootCtx)
		if err != nil {
			lgr.Logger.Error(
				"shutting down OpenTelemetry",
				slog.Any("error", xerrors.New(err.Error())),
			)
		}
	}()

	app, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		lgr.Logger.Error(
			"initializing firebase app",
			slog.Any("error", xerrors.New(err.Error())),
		)
		return
	}

	db, err := app.Firestore(rootCtx)
	if err != nil {
		lgr.Logger.Error(
			"acquiring firestore client",
			slog.Any("error", xerrors.New(err.Error())),
		)
		return
	}

	// Create an error stream
	errorStream := make(chan error)
	defer close(errorStream)

	// Create a completion stream
	completionStream := make(chan error)
	defer close(completionStream)

	mode := "monitor"
	args := os.Args[1:]
	if len(args) > 0 {
		mode = args[0]
	}

	// Determine the mode processor
	proc, ok := modeProcs[mode]
	if !ok {
		lgr.Logger.Error(
			"setting up mode processor",
			slog.Any("error", xerrors.New(err.Error())),
		)
		return
	}

	// Run the mode processor
	go func() {
		err := proc(canxCtx, app, db, errorStream)
		if err != nil {
			errorStream <- err
		}
		completionStream <- err
	}()

	// Run the http server
	go func() {
		err = server.Run(canxCtx, errorStream, os.Getenv("APP_PORT"))
		if err != nil {
			errorStream <- err
		}
	}()

	// Wait for cancellation, completion or error
	for {
		select {
		case <-canxCtx.Done():
			lgr.Logger.Info(
				"main context cancelled",
			)
			goto resume
		case e := <-completionStream:
			if e != nil {
				lgr.Logger.Error(
					"main mode prod completed with error",
					slog.Any("error", xerrors.New(e.Error())),
				)
				goto resume
			}

			lgr.Logger.Info(
				"main mode proc completed without error",
			)
			goto resume
		case e := <-errorStream:
			lgr.Logger.Error(
				"error received on stream",
				slog.Any("error", xerrors.New(e.Error())),
			)
		}
	}

	// Wait in a non-blocking way 4 seconds for all the go routines to exit
	// This is needed because the go routines may need to report as they are existing
resume:
	// Cancel the context if not already cancelled
	if canxCtx.Err() == nil {
		// Force cancel the context
		canxFn()
	}

	lgr.Logger.Info(
		"main is waiting for all go routines to exit",
	)

	timer := time.NewTimer(waitOnShutdown)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			// Timer expired, proceed with shutdown
			lgr.Logger.Info(
				"main shutdown waiting period expired. Exiting now",
				slog.Duration("period", waitOnShutdown),
			)
			return
		case e := <-completionStream:
			if e != nil {
				lgr.Logger.Error(
					"main mode prod completed with error",
					slog.Any("error", xerrors.New(e.Error())),
				)
				return
			}

			lgr.Logger.Info(
				"main mode proc completed without error",
			)
			return
		case e := <-errorStream:
			// Handle error received on errorStream
			lgr.Logger.Error(
				"error received on stream",
				slog.Any("error", xerrors.New(e.Error())),
			)
		}
	}
}

// Reference:
// https://cloud.google.com/stackdriver/docs/instrumentation/setup/go
// setupOpenTelemetry sets up the OpenTelemetry SDK and exporters for metrics and
// traces. If it does not return an error, call shutdown for proper cleanup.
func setupOpenTelemetry(ctx context.Context) (shutdown func(context.Context) error, err error) {
	if os.Getenv("DISABLE_TELEMETRY") == "true" {
		// Set Noop Tracer Provider
		otel.SetTracerProvider(nooptrace.NewTracerProvider())

		// Set Noop Meter Provider
		otel.SetMeterProvider(noopmeter.NewMeterProvider())

		// Return a no-op shutdown function
		return func(_ context.Context) error {
			return nil
		}, nil
	}

	var shutdownFuncs []func(context.Context) error

	// shutdown combines shutdown functions from multiple OpenTelemetry
	// components into a single function.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// Configure Context Propagation to use the default W3C traceparent format
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

	// Configure Trace Export to send spans as OTLP
	texporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		err = errors.Join(err, shutdown(ctx))
		return
	}
	tp := trace.NewTracerProvider(trace.WithBatcher(texporter))
	shutdownFuncs = append(shutdownFuncs, tp.Shutdown)
	otel.SetTracerProvider(tp)

	// Configure Metric Export to send metrics as OTLP
	mreader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		err = errors.Join(err, shutdown(ctx))
		return
	}
	mp := metric.NewMeterProvider(
		metric.WithReader(mreader),
	)
	shutdownFuncs = append(shutdownFuncs, mp.Shutdown)
	otel.SetMeterProvider(mp)

	return shutdown, nil
}
