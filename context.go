package eventor

import (
	"context"
	"fmt"
)

// Data holds custom data which the EventHandler can work on
type Data map[string]any

func (ed Data) Exists(key string) bool {
	_, found := ed[key]
	return found
}

func (ed Data) Get(key string) any {
	if v, found := ed[key]; found {
		return v
	}
	return nil
}

func (ed Data) Set(key string, val any) {
	ed[key] = val
}

type beforeCallback func(ctx *EventContext) error

type afterCallback func(ctx *EventContext)

type recoverCallback = func(ctx *EventContext, panicValue any)

// EventContext stores internal information
type EventContext struct {
	// Name of the triggered event
	eventName string
	// HandlerID of the current handler that gets executed
	handlerID       string
	iterations      uint64
	callStack       CallStack
	eventSourceMap  map[string]uint64
	err             error
	StopPropagation bool
	Context         context.Context
	Data            Data
}

// NewEventContext returns a new EventContext necessary to trigger/run a event
func NewEventContext(goCtx context.Context) *EventContext {
	ctx := &EventContext{}
	ctx.Context = goCtx
	ctx.eventSourceMap = make(map[string]uint64)
	ctx.Data = make(Data)
	return ctx
}

// NewEventContext returns a new EventContext necessary to trigger/run a event
func NewEventContextWithData(goCtx context.Context, data map[string]any) *EventContext {
	ctx := &EventContext{}
	ctx.Context = goCtx
	ctx.eventSourceMap = make(map[string]uint64)
	ctx.Data = data
	return ctx
}

func (ctx *EventContext) EventName() string {
	return ctx.eventName
}

func (ctx *EventContext) HandlerID() string {
	return ctx.handlerID
}

func (ctx *EventContext) pushCallStack(e string) {
	ctx.callStack = append(ctx.callStack, e)
}

func (ctx *EventContext) Err() error {
	return ctx.err
}

func (ctx *EventContext) addEventSource(e string, allowRecursion bool) error {
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
