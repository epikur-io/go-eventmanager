package eventor

import (
	"context"
	"fmt"
)

// EventData holds custom data which the EventHandler can work on
type EventData map[string]any

func (ed EventData) Exists(key string) bool {
	_, found := ed[key]
	return found
}

func (ed EventData) Get(key string) any {
	if v, found := ed[key]; found {
		return v
	}
	return nil
}

func (ed EventData) Set(key string, val any) {
	ed[key] = val
}

type beforeCallback func(ctx *EventCtx) error

type afterCallback func(ctx *EventCtx)

type panicRecoverCallback = func(ctx *EventCtx, panicValue any)

// EventCtx stores internal information
type EventCtx struct {
	// Name of the triggered event
	eventName string
	// HandlerID of the current handler that gets executed
	handlerID       string
	GoContext       context.Context
	iterations      uint64
	callStack       CallStack
	eventSourceMap  map[string]uint64
	StopPropagation bool
	err             error
	Data            EventData
}

// NewEventContext returns a new EventCtx necessary to trigger/run a event
func NewEventContext(goCtx context.Context) *EventCtx {
	ctx := &EventCtx{}
	ctx.GoContext = goCtx
	ctx.eventSourceMap = make(map[string]uint64)
	ctx.Data = make(map[string]interface{})
	return ctx
}

func (ctx *EventCtx) EventName() string {
	return ctx.eventName
}

func (ctx *EventCtx) HandlerID() string {
	return ctx.handlerID
}

func (ctx *EventCtx) pushCallStack(e string) {
	ctx.callStack = append(ctx.callStack, e)
}

func (ctx *EventCtx) Err() error {
	return ctx.err
}

func (ctx *EventCtx) addEventSource(e string, allowRecursion bool) error {
	if _, ok := ctx.eventSourceMap[e]; ok {
		ctx.eventSourceMap[e] += 1
		if !allowRecursion && ctx.eventSourceMap[e] > 1 {
			return fmt.Errorf("event source \"%s\" already exists", e)
		}
	}
	ctx.eventSourceMap[e] = 1
	return nil
}

type CallStack []string

func (c CallStack) Contains(callerID string) bool {
	for _, entry := range c {
		if entry == callerID {
			return true
		}
	}
	return false
}
