package main

import (
	"context"
	"log"

	"github.com/epikur-io/go-eventor"
)

type User struct {
	ID    int64
	Name  string
	Email string
}

func main() {
	// Create a new observer with default options
	observer := eventor.NewObserver()

	// Add event handlers
	observer.AddEventHandler(eventor.EventHandler{
		Name: "user.created",
		ID:   "send_welcome_email",
		Prio: 50,
		Func: func(ctx *eventor.EventCtx) {
			user := ctx.Data.Get("user").(*User)
			log.Printf("Sending welcome email to: %s", user.Email)
		},
	})

	// Trigger event
	ectx := eventor.NewEventContext(context.Background())
	ectx.Data.Set("user", &User{ID: 1, Name: "user", Email: "user@example.com"})

	count, err := observer.Trigger("user.created", ectx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Executed %d handlers", count)
}
