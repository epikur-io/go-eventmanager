package eventor

// up-to-date package with working recursion detection (2025-02-24)

import (
	"errors"
	"fmt"
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

type SortOrder int

const (
	ExecAscending SortOrder = iota
	ExecDescending
)

func (so SortOrder) String() string {
	switch so {
	case ExecAscending:
		return "ASC"
	case ExecDescending:
		return "DESC"
	default:
		return "UNKNOWN"
	}
}

type EventDispatcher interface {
	AllowRecursion(allow bool)
	SetCallstackLimit(size int)

	Dispatch(eventName string, ctx *EventContext) (uint64, error)
	DispatchSafe(eventName string, ctx *EventContext) uint64

	Handlers() map[string]HandlerList
	AddAll(eventHandlers []Handler, opt ...bool) error
	Add(eventHandler Handler, opt ...bool) error
	Clear()
	ClearEvent(eventName string)
	Remove(eventName string, id string)
	RemoveID(id string) uint64
	RemoveIDPrefix(prefix string) uint64

	CountID(id string) uint64
	CountIDPrefix(prefix string) uint64
	CountEventAndID(eventName string, id string) uint64
	CountEventAndIDPrefix(eventName string, prefix string) uint64
}

// esnure our implementation satisfies the Observer interface
var _ EventDispatcher = &Observer{}

type config struct {
	logger               zerolog.Logger
	callstackLimit       int  // The max number of callback per event
	allowRecursion       bool // disabled by default
	hasLog               bool
	panicRecoverCallback panicRecoverCallback
	beforeCallback       beforeCallback
	afterCallback        afterCallback
	executionOrder       SortOrder
}

type option func(*config)

func WithLogger(l zerolog.Logger) option {
	return func(c *config) {
		c.logger = l
		c.hasLog = true
	}
}

func WithPanicRecover(fn panicRecoverCallback) option {
	return func(c *config) {
		c.panicRecoverCallback = fn
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

func WithBeforeCallback(f beforeCallback) option {
	return func(c *config) {
		c.beforeCallback = f
	}
}

func WithAfterCallback(f afterCallback) option {
	return func(c *config) {
		c.afterCallback = f
	}
}

func WithExecutionOrder(order SortOrder) option {
	return func(c *config) {
		c.executionOrder = order
	}
}

// Observer manages the hanlder chains and dispatching of events.
type Observer struct {
	eventHandlers map[string]HandlerList // Event handler map
	config        config
	mux           *sync.RWMutex
}

func NewObserver(opts ...option) *Observer {
	o := &Observer{
		config: config{
			executionOrder: ExecDescending,
		},
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
	o.eventHandlers = make(map[string]HandlerList)
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
func (o *Observer) Handlers() map[string]HandlerList {
	o.mux.RLock()
	var meta map[string]HandlerList = make(map[string]HandlerList)
	for e, ec := range o.eventHandlers {
		meta[e] = HandlerList{}
		copy(meta[e], ec[:])
	}
	o.mux.RUnlock()
	return meta
}

// Deletes all registered event handlers
func (o *Observer) Clear() {
	o.mux.Lock()
	o.deleteAll()
	o.mux.Unlock()
}

func (m *Observer) deleteAll() {
	m.eventHandlers = make(map[string]HandlerList)
}

// DeleteByEvent deletes event handler by its event name their listening on
func (o *Observer) ClearEvent(ei string) {
	o.mux.Lock()
	o.clearEvent(ei)
	o.mux.Unlock()
}

func (o *Observer) clearEvent(ei string) {
	delete(o.eventHandlers, ei)
}

// Deletes the registered event handlers filtered by event name and ID
func (o *Observer) Remove(ei string, id string) {
	o.mux.Lock()
	o.remove(ei, id)
	o.mux.Unlock()
}

func (o *Observer) remove(ei string, id string) {
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
	o.eventHandlers[ei].Sort(o.config.executionOrder)
}

// Deletes all event handlers with the given ID ignoring the event name
func (o *Observer) RemoveID(id string) uint64 {
	o.mux.Lock()
	deleted := o.removeID(id)
	o.mux.Unlock()
	return deleted
}
func (o *Observer) removeID(id string) uint64 {
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
		o.eventHandlers[ei].Sort(o.config.executionOrder)
	}
	return uint64(total)
}

// Deletes all event handlers that where the ID starts with the given prefix
func (o *Observer) RemoveIDPrefix(prefix string) uint64 {
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
		o.eventHandlers[ei].Sort(o.config.executionOrder)
	}
	o.mux.Unlock()
	return uint64(total)
}

// Returns the number of event handlers registers with the given ID ignoring the event name
func (o *Observer) CountID(id string) uint64 {
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
func (o *Observer) CountIDPrefix(prefix string) uint64 {
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
func (o *Observer) CountEventAndID(event string, id string) uint64 {
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

func (o *Observer) CountEventAndIDPrefix(event string, prefix string) uint64 {
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

func (o *Observer) AddAll(es []Handler, opt ...bool) error {
	o.mux.Lock()
	defer o.mux.Unlock()
	errList := []string{}
	for _, e := range es {
		// // automatic order
		// if e.Order == 0 {
		// 	e.Order = i
		// }
		err := o.add(&e, opt...)
		if err != nil {
			errList = append(errList, err.Error())
		}
	}
	if len(errList) > 0 {
		return fmt.Errorf("failed to add event handlers, got errors:\n%s", strings.Join(errList, "\n"))
	}
	return nil
}

func (o *Observer) Add(e Handler, opt ...bool) error {
	o.mux.Lock()
	defer o.mux.Unlock()
	return o.add(&e, opt...)
}

// AddEventHandler adds an event handler to the event Manager
// the provided function must not start goroutines that trigger other events
// that use references of the event or event context!
func (o *Observer) add(e *Handler, opt ...bool) error {
	// !TODO remove "opt" parameter or rename it to prefixCheck (better naming)
	if e == nil {
		return fmt.Errorf("missing event handler")
	}
	if e.Func == nil {
		return fmt.Errorf("missing function for event handler")
	}
	_, ok := o.eventHandlers[e.Name]
	if !ok {
		o.eventHandlers[e.Name] = []*Handler{}
	}
	if len(opt) > 0 && opt[0] {
		var prefixCheck bool
		if len(opt) > 1 {
			prefixCheck = opt[1]
		}
		for _, xe := range o.eventHandlers[e.Name] {
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
	}
	o.eventHandlers[e.Name] = append(o.eventHandlers[e.Name], e)
	o.eventHandlers[e.Name].Sort(o.config.executionOrder)
	return nil
}

func (o *Observer) ReplaceHandlersByID(es []Handler, opt ...bool) {
	o.mux.Lock()
	defer o.mux.Unlock()
	for _, e := range es {
		o.RemoveID(e.ID)
		_ = o.add(&e, opt...)
	}
}
func (o *Observer) ReplaceHandlersByEventAndID(es []Handler, opt ...bool) {
	o.mux.Lock()
	defer o.mux.Unlock()
	for _, e := range es {
		o.remove(e.Name, e.ID)
		_ = o.add(&e, opt...)
	}
}

// Triggers an event with the given data and context and logs
// potential errors but doesn't return them
func (o *Observer) DispatchSafe(name string, ctx *EventContext) uint64 {
	o.mux.RLock()
	defer o.mux.RUnlock()
	res, err := o.dispatch(name, ctx)

	if err != nil && o.config.hasLog {
		o.config.logger.Error().
			Str("event", name).
			Err(err).
			Msg("event execution failed")
	}
	return res

}

// Triggers an event with the given data and context
func (o *Observer) Dispatch(name string, ctx *EventContext) (uint64, error) {
	o.mux.RLock()
	defer o.mux.RUnlock()
	return o.dispatch(name, ctx)
}

func (o *Observer) dispatch(name string, ctx *EventContext) (uint64, error) {
	if ctx == nil {
		return 0, ErrMissingEventCtx
	}
	if ctx.err != nil {
		return ctx.iterations, ctx.err
	}
	el, ok := o.eventHandlers[name]
	if !ok || len(el) < 1 {
		return ctx.iterations, ctx.err
	}
	currentEventName := name
	if ctx.EventName() == "" {
		ctx.eventName = name
	}
	if err := ctx.addEventSource(currentEventName, o.config.allowRecursion); err != nil {
		// also checks for recursion and stops if not allowed
		ctx.err = err
		return ctx.iterations, ctx.err
	}
	if o.config.beforeCallback != nil {
		if err := o.config.beforeCallback(ctx); err != nil {
			return ctx.iterations, err
		}
	}
	defer func() {
		if o.config.panicRecoverCallback != nil {
			if r := recover(); r != nil {
				if o.config.hasLog {
					o.config.logger.Error().
						Str("event", ctx.eventName).
						Str("handler_id", ctx.handlerID).
						Any("panic_value", r).
						Msg("recoverd from panic.")
				}
				o.config.panicRecoverCallback(ctx, r)
			}
		}
		if o.config.afterCallback != nil {
			o.config.afterCallback(ctx)
		}
	}()
	for _, e := range el {
		if ctx.Context != nil {
			select {
			case <-ctx.Context.Done():
				return ctx.iterations, ctx.Context.Err()
			default:
			}
		}
		if ctx.err != nil {
			return ctx.iterations, ctx.err
		}
		// using address of the event handler as unique ID
		handlerID := fmt.Sprintf("%p", e)
		if !o.config.allowRecursion && ctx.callStack.Contains(handlerID) {
			ctx.err = ErrRecursionNotAllowed
			return ctx.iterations, ctx.err
		}
		if o.config.callstackLimit > 0 && len(ctx.callStack) >= o.config.callstackLimit {
			ctx.err = ErrCallstackLimitExeeded
			return ctx.iterations, ctx.err
		}
		if ctx.StopPropagation {
			return ctx.iterations, ctx.err
		}
		if o.config.hasLog {
			o.config.logger.Debug().
				Str("event", e.Name).
				Str("event_id", e.ID).
				Str("handler_id", handlerID).
				Msg("executing event handler")
		}
		ctx.pushCallStack(handlerID)
		ctx.handlerID = e.ID
		sourceEventName := ctx.eventName
		ctx.eventName = currentEventName
		e.Func(ctx)
		if sourceEventName != "" {
			ctx.eventName = sourceEventName
		}
		ctx.iterations += 1
	}
	return ctx.iterations, ctx.err
}
