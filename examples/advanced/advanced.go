package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	evm "github.com/epikur-io/go-eventmanager"
	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	observer := evm.NewObserver(
		// default execution order is descending, highest prio first,
		// use `evm.ExecAscending` for reverse order.
		evm.WithExecutionOrder(evm.ExecDescending),
		evm.WithPanicRecover(func(ctx *evm.EventCtx, panicValue any) {
			log.Printf("recovered from panic caused by: %s (ID: %s), panic value: %v\n", ctx.EventName(), ctx.HandlerID(), panicValue)
		}),
		evm.WithLogger(logger),
		evm.WithBeforeCallback(func(ctx *evm.EventCtx) error {
			// This callback runs before any event handler chain gets executed.
			// This could be used to inject context information based on the given event `ctx.EventName()`
			// or other parameters.
			ctx.Data.Set("Hello", "World")
			return nil
		}),
		evm.WithAfterCallback(func(ctx *evm.EventCtx) {
			// This callback runs after the event handler chain was executed.
			// This can be used to cleanup things to trigger further program logic.
			if v := ctx.Data.Get("datetime"); v != nil {
				fmt.Println("(AfterCallback) Datetime:", v.(time.Time).String())
			}
		}),
	)

	// Add multiple event handlers / hooks
	// use "observer.AddEventHandler(evm.EventHandler{..})" to add a  single event handle
	err := observer.AddHandlers([]evm.EventHandler{
		// Multiple handlers for the same event, handlers will be executed by their given order
		{
			EventName: "event_a",
			ID:        "first_handler", // Unique identifier for this event handler (useful for logging & debugging)
			Prio:      30,              // Priority for event handler execution (highest first)
			Func: func(ctx *evm.EventCtx) {
				log.Printf("executing event handler: %s (ID: %s)\n", ctx.EventName(), ctx.HandlerID())

				// add some custom data to the context
				ctx.Data["datetime"] = time.Now()
			},
		},
		{
			EventName: "event_a",
			ID:        "second_handler",
			Prio:      20,
			Func: func(ctx *evm.EventCtx) {
				log.Printf("executing event handler: %s (ID: %s)\n", ctx.EventName(), ctx.HandlerID())

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
			Prio:      10,
			Func: func(ctx *evm.EventCtx) {
				// this should not be executed because `ctx.StopPropagation` was set to true.
				log.Printf("executing event handler: %s (ID: %s)\n", ctx.EventName(), ctx.HandlerID())
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
	cnt, err := observer.Trigger("event_a", ectx) // calling `Trigger(...)` sequentially executes all event handlers for `event_a` ordered descending by the event handlers' priority.
	if err != nil {
		log.Panic(err)
	}
	log.Printf("Executed %d handlers\n", cnt)
	log.Printf("Datetime:  %v\n", ectx.Data["datetime"])
}
