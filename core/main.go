package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	firebase "firebase.google.com/go/v4"

	"github.com/khaledhikmat/family-meeting/mode"
	"github.com/khaledhikmat/family-meeting/mode/broadcast"
	"github.com/khaledhikmat/family-meeting/mode/monitor"
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

	mode := "monitor"
	args := os.Args[1:]
	if len(args) > 0 {
		mode = args[0]
	}

	// Run the mode processor
	if proc, ok := modeProcs[mode]; ok {
		go func() {
			err := proc(canxCtx, app, db, errorStream)
			if err != nil {
				errorStream <- fmt.Errorf("mode processor error: %v", err)
			}
		}()
	}

	// Wait for cancellation
	<-canxCtx.Done()
	fmt.Printf("main context cancelled: %v\n", canxCtx.Err())
	// Wait 4 seconds for all the go routines to exit
	time.Sleep(4 * time.Second)
}

func streamError(canxCtx context.Context, errorStream chan error) {
	for {
		select {
		case <-canxCtx.Done():
			fmt.Printf("streamError context cancelled: %v\n", canxCtx.Err())
			return
		case e, ok := <-errorStream:
			if !ok {
				return
			}
			fmt.Printf("streamError processor processed this error: %v\n", e)
		}
	}
}
