package mode

import (
	"context"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
)

// Signature of mode processors
type Processor func(canxCtx context.Context,
	app *firebase.App,
	db *firestore.Client,
	errorStream chan error) error
