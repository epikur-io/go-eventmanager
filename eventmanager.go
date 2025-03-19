package eventmanager

// up-to-date package with working recursion detection (2025-02-24)

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// A thread-safe event manager using the observer pattern.
// Your can register multiple EventHandler's on an event name.
// EventHandler's are executed in user defined order.
// For security reasons the EventManage doesn't allow recursive event calls like:
// eventA triggers -> eventA   or:
// eventA triggers -> eventB tiggers -> eventA ... (eventB triggered eventA but eventA was already on callstack!)
// Each event has its on context when a event is triggered
// the context stores a call-chain. If a event name occurs more than once
// a "recursion not allowed" error is returned.

const (
	DefaultCallstackLimit = uint64(510) // 2 * 255
)

var (
	ErrRecursionNotAllowed   = errors.New("recursion is not allowed")
	ErrCallstackLimitExeeded = errors.New("callstack limit exceeded")
	ErrMissingEventCtx       = errors.New("missing event context")
)

type CallStack []string

func (c CallStack) Contains(callerID string) bool {
	for _, entry := range c {
		if entry == callerID {
			return true
		}
	}
	return false
}

// EventData holds custom data which the EventHandler can work on
type EventData map[string]any

func (ed EventData) Get(key string) any {
	if v, found := ed[key]; found {
		return v
	}
	return nil
}

func (ed EventData) Set(key string, val any) {
	ed[key] = val
}

// EventCtx stores internal information
type EventCtx struct {
	// Name of the triggered event
	EventName string
	// HandlerID of the current handler that gets executed
	HandlerID       string
	GoContext       context.Context
	Interations     uint64
	CallStack       CallStack
	EventSourceMap  map[string]uint64
	StopPropagation bool
	err             error
	Data            EventData
}

// NewEventContext returns a new EventCtx necessary to trigger/run a event
func NewEventContext(goCtx context.Context) *EventCtx {
	ctx := &EventCtx{}
	ctx.GoContext = goCtx
	ctx.EventSourceMap = make(map[string]uint64)
	ctx.Data = make(map[string]interface{})
	return ctx
}

func (ctx *EventCtx) pushCallStack(e string) {
	ctx.CallStack = append(ctx.CallStack, e)
}

func (ctx *EventCtx) Error() error {
	return ctx.err
}

func (ctx *EventCtx) addEventSource(e string, allowRecursion bool) error {
	if _, ok := ctx.EventSourceMap[e]; ok {
		ctx.EventSourceMap[e] += 1
		if !allowRecursion && ctx.EventSourceMap[e] > 1 {
			return fmt.Errorf("event source \"%s\" already exists", e)
		}
	}
	ctx.EventSourceMap[e] = 1
	return nil
}

// EventHandler defines a event handler thats listening on one specific event
type EventHandler struct {
	EventName string          `json:"name"` // EventName
	Order     int             `json:"order"`
	Func      func(*EventCtx) `json:"-"`
	ID        string          `json:"id"`
}

// EventHandlerList is a list of event handlers that provides a sorting interface
type EventHandlerList []*EventHandler

func (s EventHandlerList) Sort() {
	sort.Sort(s)
}
func (s EventHandlerList) Len() int {
	return len(s)
}
func (s EventHandlerList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s EventHandlerList) Less(i, j int) bool {
	return (s[i].Order) < (s[j].Order)
}

type IObserver interface {
	AllowRecursion(allow bool)
	SetCallstackLimit(size int)
	RegisteredHandlers() map[string]EventHandlerList
	DeleteAll()
	DeleteByEvent(eventName string)
	DeleteByEventAndID(eventName string, id string)
	DeleteByID(id string) uint64
	DeleteByIDPrefix(prefix string) uint64
	CountByID(id string) uint64
	CountByIDPrefix(prefix string) uint64
	CountByEventAndID(eventName string, id string) uint64
	CountByEventAndIDPrefix(eventName string, prefix string) uint64
	AddHandlers(eventHandlers []EventHandler, opt ...bool) error
	AddEventHandler(eventHandler EventHandler, opt ...bool) error
	Trigger(eventName string, ctx *EventCtx) (uint64, error)
	TriggerCatch(eventName string, ctx *EventCtx) uint64
}

// esnure our implementation satisfies the Observer interface
var _ IObserver = &Observer{}

type config struct {
	logger         zerolog.Logger
	callstackLimit int  // The max number of callback per event
	allowRecursion bool // disabled by default
	hasLog         bool
}

type option func(*config)

func WithLogger(l zerolog.Logger) option {
	return func(c *config) {
		c.logger = l
		c.hasLog = true
	}
}

func WithCallstackLimit(l int) option {
	return func(c *config) {
		c.callstackLimit = l
	}
}

func WithRecursionAllowed() option {
	return func(c *config) {
		c.allowRecursion = true
	}
}

// Observer manages the event/event chains
type Observer struct {
	eventHandlers map[string]EventHandlerList // Event handler map
	config        config
	mux           *sync.RWMutex
}

func NewObserver(opts ...option) *Observer {
	o := &Observer{
		config: config{},
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&o.config)
		}
	}

	o.init()
	return o
}

// Initalizes the EventManager and must be called befor you use it when not using the constructor
func (o *Observer) init() {
	o.mux = &sync.RWMutex{}
	o.eventHandlers = make(map[string]EventHandlerList)
	if o.config.callstackLimit < 1 {
		o.config.callstackLimit = int(DefaultCallstackLimit)
	}
}

// Allows recursive executuion of event handlers debending on the setting
func (o *Observer) AllowRecursion(allow bool) {
	o.mux.Lock()
	o.config.allowRecursion = allow
	o.mux.Unlock()
}

// Allows recursive executuion of event handlers debending on the setting
func (o *Observer) SetCallstackLimit(size int) {
	o.mux.Lock()
	o.config.callstackLimit = size
	o.mux.Unlock()
}

// Returns all registered event handlers mapped by eventName->Handler
func (o *Observer) RegisteredHandlers() map[string]EventHandlerList {
	o.mux.RLock()
	var meta map[string]EventHandlerList = make(map[string]EventHandlerList)
	for e, ec := range o.eventHandlers {
		meta[e] = EventHandlerList{}
		copy(meta[e], ec[:])
	}
	o.mux.RUnlock()
	return meta
}

// Deletes all registered event handlers
func (o *Observer) DeleteAll() {
	o.mux.Lock()
	o.deleteAll()
	o.mux.Unlock()
}

func (m *Observer) deleteAll() {
	m.eventHandlers = make(map[string]EventHandlerList)
}

// DeleteByEvent deletes event handler by its event name their listening on
func (o *Observer) DeleteByEvent(ei string) {
	o.mux.Lock()
	o.deleteByEvent(ei)
	o.mux.Unlock()
}

func (o *Observer) deleteByEvent(ei string) {
	delete(o.eventHandlers, ei)
}

// Deletes the registered event handlers filtered by event name and ID
func (o *Observer) DeleteByEventAndID(ei string, id string) {
	o.mux.Lock()
	o.deleteByEventAndID(ei, id)
	o.mux.Unlock()
}

func (o *Observer) deleteByEventAndID(ei string, id string) {
	ev, ok := o.eventHandlers[ei]
	if !ok || len(ev) < 1 {
		return
	}
	deleted := 0
	for i, e := range ev {
		j := i - deleted
		if e.ID == id {
			if len(o.eventHandlers[ei]) == 1 {
				delete(o.eventHandlers, ei)
				break
			}
			o.eventHandlers[ei] = append(
				o.eventHandlers[ei][:j],
				o.eventHandlers[ei][j+1:]...,
			)
			deleted++
		}
	}
	o.eventHandlers[ei].Sort()
}

// Deletes all event handlers with the given ID ignoring the event name
func (o *Observer) DeleteByID(id string) uint64 {
	o.mux.Lock()
	deleted := o.deleteByID(id)
	o.mux.Unlock()
	return deleted
}
func (o *Observer) deleteByID(id string) uint64 {
	total := 0
	for ei, ev := range o.eventHandlers {
		deleted := 0
		for i, e := range ev {
			j := i - deleted
			if e.ID == id {
				if len(o.eventHandlers[ei]) == 1 {
					delete(o.eventHandlers, ei)
					deleted++
					break
				}
				o.eventHandlers[ei] = append(
					o.eventHandlers[ei][:j], o.eventHandlers[ei][j+1:]...,
				)
				deleted++
			}
		}
		total += deleted
		o.eventHandlers[ei].Sort()
	}
	return uint64(total)
}

// Deletes all event handlers that where the ID starts with the given prefix
func (o *Observer) DeleteByIDPrefix(prefix string) uint64 {
	o.mux.Lock()
	total := 0
	for ei, ev := range o.eventHandlers {
		deleted := 0
		for i, e := range ev {
			j := i - deleted
			if strings.HasPrefix(e.ID, prefix) {
				if len(o.eventHandlers[ei]) == 1 {
					delete(o.eventHandlers, ei)
					deleted++
					break
				}
				o.eventHandlers[ei] = append(
					o.eventHandlers[ei][:j], o.eventHandlers[ei][j+1:]...,
				)
				deleted++
			}
		}
		total += deleted
		o.eventHandlers[ei].Sort()
	}
	o.mux.Unlock()
	return uint64(total)
}

// Returns the number of event handlers registers with the given ID ignoring the event name
func (o *Observer) CountByID(id string) uint64 {
	o.mux.RLock()
	found := uint64(0)
	for _, ev := range o.eventHandlers {
		for _, e := range ev {
			if e.ID == id {
				found++
			}
		}
	}
	o.mux.RUnlock()
	return found
}

// Returns the number of registered event handlers filterd by ID ignoring the name
func (o *Observer) CountByIDPrefix(prefix string) uint64 {
	o.mux.RLock()
	found := uint64(0)
	for _, ev := range o.eventHandlers {
		for _, e := range ev {
			if strings.HasPrefix(e.ID, prefix) {
				found++
			}
		}
	}
	o.mux.RUnlock()
	return found
}

// Returns the number of registered event handlers filterd by name and ID
func (o *Observer) CountByEventAndID(event string, id string) uint64 {
	o.mux.RLock()
	defer o.mux.RUnlock()
	if _, ok := o.eventHandlers[event]; !ok {
		return 0
	}
	found := uint64(0)
	for _, e := range o.eventHandlers[event] {
		if e.ID == id {
			found++
		}
	}
	return found
}

func (o *Observer) CountByEventAndIDPrefix(event string, prefix string) uint64 {
	o.mux.RLock()
	defer o.mux.RUnlock()
	if _, ok := o.eventHandlers[event]; !ok {
		return 0
	}
	found := uint64(0)
	for _, e := range o.eventHandlers[event] {
		if strings.HasPrefix(e.ID, prefix) {
			found++
		}
	}
	return found
}

func (o *Observer) AddHandlers(es []EventHandler, opt ...bool) error {
	o.mux.Lock()
	defer o.mux.Unlock()
	errList := []string{}
	for _, e := range es {
		err := o.addHandler(&e, opt...)
		if err != nil {
			errList = append(errList, err.Error())
		}
	}
	if len(errList) > 0 {
		return fmt.Errorf("failed to add event handlers, got errors:\n%s", strings.Join(errList, "\n"))
	}
	return nil
}

func (o *Observer) AddEventHandler(e EventHandler, opt ...bool) error {
	o.mux.Lock()
	defer o.mux.Unlock()
	return o.addHandler(&e, opt...)
}

// AddEventHandler adds an event handler to the event Manager
// the provided function must not start goroutines that trigger other events
// that use references of the event or event context!
func (o *Observer) addHandler(e *EventHandler, opt ...bool) error {
	// !TODO remove "opt" parameter or rename it to prefixCheck (better naming)
	if e == nil {
		return fmt.Errorf("missing event handler")
	}
	if e.Func == nil {
		return fmt.Errorf("missing function for event handler")
	}
	_, ok := o.eventHandlers[e.EventName]
	if !ok {
		o.eventHandlers[e.EventName] = []*EventHandler{}
	}
	if len(opt) > 0 && opt[0] {
		var prefixCheck bool
		if len(opt) > 1 {
			prefixCheck = opt[1]
		}
		for _, xe := range o.eventHandlers[e.EventName] {
			if prefixCheck {
				if strings.HasPrefix(xe.ID, e.ID) {
					return fmt.Errorf("event handler \"%s\" with the same id prefix \"%s\" is already registered", xe.ID, e.ID)
				}
			} else {
				if xe.ID == e.ID {
					return fmt.Errorf("event handler with the same id \"%s\" is already registered", e.ID)
				}
			}
		}
		o.eventHandlers[e.EventName] = append(o.eventHandlers[e.EventName], e)
		o.eventHandlers[e.EventName].Sort()
	} else {
		o.eventHandlers[e.EventName] = append(o.eventHandlers[e.EventName], e)
		o.eventHandlers[e.EventName].Sort()
	}
	return nil
}

func (o *Observer) ReplaceHandlersByID(es []EventHandler, opt ...bool) {
	o.mux.Lock()
	defer o.mux.Unlock()
	for _, e := range es {
		o.deleteByID(e.ID)
		_ = o.addHandler(&e, opt...)
	}
}
func (o *Observer) ReplaceHandlersByEventAndID(es []EventHandler, opt ...bool) {
	o.mux.Lock()
	defer o.mux.Unlock()
	for _, e := range es {
		o.deleteByEventAndID(e.EventName, e.ID)
		_ = o.addHandler(&e, opt...)
	}
}

// Triggers an event with the given data and context and logs
// potential errors but doesn't return them
func (o *Observer) TriggerCatch(name string, ctx *EventCtx) uint64 {
	o.mux.RLock()
	defer o.mux.RUnlock()
	res, err := o.trigger(name, ctx)

	if err != nil && o.config.hasLog {
		o.config.logger.Error().
			Str("event", name).
			Err(err).
			Msg("event execution failed")
	}
	return res

}

// Triggers an event with the given data and context
func (o *Observer) Trigger(name string, ctx *EventCtx) (uint64, error) {
	o.mux.RLock()
	defer o.mux.RUnlock()
	return o.trigger(name, ctx)
}

func (o *Observer) trigger(name string, ctx *EventCtx) (uint64, error) {
	if ctx == nil {
		return 0, ErrMissingEventCtx
	}
	if ctx.err != nil {
		return ctx.Interations, ctx.err
	}
	el, ok := o.eventHandlers[name]
	if !ok || len(el) < 1 {
		return ctx.Interations, ctx.err
	}
	ctx.EventName = name
	if err := ctx.addEventSource(name, o.config.allowRecursion); err != nil {
		// also checks for recursion and stops if not allowed
		ctx.err = err
		return ctx.Interations, ctx.err
	}
	for _, e := range el {
		if ctx.GoContext != nil {
			select {
			case <-ctx.GoContext.Done():
				return ctx.Interations, ctx.GoContext.Err()
			default:
			}
		}
		if ctx.err != nil {
			return ctx.Interations, ctx.err
		}
		// using address of the event handler as unique ID
		handlerID := fmt.Sprintf("%p", e)
		if !o.config.allowRecursion && ctx.CallStack.Contains(handlerID) {
			ctx.err = ErrRecursionNotAllowed
			return ctx.Interations, ctx.err
		}
		if o.config.callstackLimit > 0 && len(ctx.CallStack) >= o.config.callstackLimit {
			ctx.err = ErrCallstackLimitExeeded
			return ctx.Interations, ctx.err
		}
		if ctx.StopPropagation {
			return ctx.Interations, ctx.err
		}
		if o.config.hasLog {
			o.config.logger.Debug().
				Str("event", e.EventName).
				Str("event_id", e.ID).
				Str("handler_id", handlerID).
				Msg("executing event handler")
		}
		ctx.pushCallStack(handlerID)
		ctx.HandlerID = e.ID
		e.Func(ctx)
		ctx.Interations += 1
	}
	return ctx.Interations, ctx.err
}

var _ IObserver = &ObserverMock{}

type ObserverMock struct {
	Observer
}

func (m *ObserverMock) RegisteredHandlers() map[string]EventHandlerList            { return nil }
func (m *ObserverMock) DeleteAll()                                                 {}
func (m *ObserverMock) DeleteByEvent(ei string)                                    {}
func (m *ObserverMock) DeleteByEventAndID(ei string, id string)                    {}
func (m *ObserverMock) DeleteByID(id string) uint64                                { return uint64(0) }
func (m *ObserverMock) DeleteByIDPrefix(prefix string) uint64                      { return uint64(0) }
func (m *ObserverMock) CountByID(id string) uint64                                 { return 0 }
func (m *ObserverMock) CountByIDPrefix(prefix string) uint64                       { return 0 }
func (m *ObserverMock) CountByEventAndID(event string, id string) uint64           { return 0 }
func (m *ObserverMock) CountByEventAndIDPrefix(event string, prefix string) uint64 { return 0 }
func (m *ObserverMock) AddHandlers(es []EventHandler, opt ...bool) error           { return nil }
func (m *ObserverMock) AddEventHandler(e EventHandler, opt ...bool) error          { return nil }
func (m *ObserverMock) Trigger(name string, ctx *EventCtx) (uint64, error) {
	return 0, nil
}
func (m *ObserverMock) TriggerCatch(name string, ctx *EventCtx) uint64 {
	return 0
}
