# A thread-safe event manager for Go

## Example:


```go
package main

import (
	"context"
	"log"
	"os"
	"time"

	evm "github.com/epikur-io/go-eventmanager"
	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	observer := evm.NewObserver(evm.WithLogger(logger))

	// Add multiple event handlers / hooks
	// use "observer.AddEventHandler(evm.EventHandler{..})" to add a  single event handle
	err := observer.AddHandlers([]evm.EventHandler{
		// Multiple handlers for the same event, handlers will be executed by their given order
		{
			EventName: "event_a",
			ID:        "first_handler", // Unique identifier for this event handler (useful for logging & debugging)
			Prio:     20, 				// Priority for event handler execution (highest first)
			Func: func(ctx *evm.EventCtx) {
				log.Printf("executing event handler: %s (ID: %s)\n", ctx.EventName, ctx.HandlerID)

				// add some custom data to the context
				ctx.Data["datetime"] = time.Now()
			},
		},
		{
			EventName: "event_a",
			ID:        "second_handler",
			Prio:     10,
			Func: func(ctx *evm.EventCtx) {
				log.Printf("executing event handler: %s (ID: %s)\n", ctx.EventName, ctx.HandlerID)

				// do some additional work on the data provided by the previous handler
				datetime, ok := ctx.Data["datetime"].(time.Time)
				if ok {
					ctx.Data["datetime"] = datetime.Add(time.Hour * 24)
				}

				// This will stop further execution of following event handlers
				ctx.StopPropagation = true
			},
		},
		{
			EventName: "event_a",
			ID:        "third_handler",
			Prio:     11,
			Func: func(ctx *evm.EventCtx) {
				log.Printf("executing event handler: %s (ID: %s)\n", ctx.EventName, ctx.HandlerID)
				// this should not be executed
			},
		},
	})
	if err != nil {
		log.Fatalln(err)
	}

	// context with 1 second timeout
	expiry, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	ectx := evm.NewEventContext(expiry)

	// Trigger an event
	cnt, err := observer.Trigger("event_a", ectx) // calling `Trigger(...)` sequentially executes all event handlers for `event_a` ordered ascending by the event handlers' priority. 
	if err != nil {
		log.Panic(err)
	}
	log.Printf("Executed %d handlers\n", cnt)
	log.Printf("Datetime:  %v\n", ectx.Data["datetime"])
}
```