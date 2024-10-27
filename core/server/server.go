package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/khaledhikmat/family-meeting/service/lgr"
	"github.com/mdobak/go-xerrors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type ginWithContext func(canxCtx context.Context, errorStream chan error) error

var (
	meter = otel.Meter(fmt.Sprintf("family.meeting.%s.server", os.Getenv("APP_NAME")))

	invocationCounter metric.Int64Counter
)

func init() {
	var err error
	invocationCounter, err = meter.Int64Counter(
		fmt.Sprintf("family.meeting.%s.server.invocation.counter", os.Getenv("APP_NAME")),
		metric.WithDescription(fmt.Sprintf("The number of %s server invocations", os.Getenv("APP_NAME"))),
		metric.WithUnit("1"),
	)
	if err != nil {
		lgr.Logger.Error(
			"creating counter",
			slog.Any("error", xerrors.New(err.Error())),
		)
	}
}

func Run(canxCtx context.Context, errorStream chan error, port string) error {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		invocationCounter.Add(canxCtx, 1)
		c.JSON(200, gin.H{
			"message": "pong:" + port,
		})
	})

	fn := getRunWithCanxFn(r, ":"+port)
	return fn(canxCtx, errorStream)
}

func getRunWithCanxFn(r *gin.Engine, port string) ginWithContext {
	return func(canxCtx context.Context, errorStream chan error) error {
		go func() {
			if err := r.Run(port); err != nil {
				errorStream <- fmt.Errorf("error runing gin: %v", err)
				return
			}
		}()

		// Wait until cancelled
		<-canxCtx.Done()
		return canxCtx.Err()
	}
}
