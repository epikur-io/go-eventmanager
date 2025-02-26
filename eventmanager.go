package eventmanager

// up-to-date package with working recursion detection (2025-02-24)

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
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
type EventData = map[string]any

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

func (ctx *EventCtx) addEventSource(e string, allowRecursion bool) error {
	if _, ok := ctx.EventSourceMap[e]; ok {
		ctx.EventSourceMap[e] += 1
		if !allowRecursion && ctx.EventSourceMap[e] > 1 {
			return errors.New("event source already exists")
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
type EventHandlerList []EventHandler

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

type Observer interface {
	AllowRecursion(allow bool)
	RegisteredHandlers() map[string]EventHandlerList
	DeleteAll()
	DeleteByEvent(ei string)
	DeleteByEventAndID(ei string, id string)
	DeleteByID(id string)
	DeleteByIDPrefix(prefix string)
	CountByID(id string) uint64
	CountByIDPrefix(prefix string) uint64
	CountByEventAndID(event string, id string) uint64
	CountByEventAndIDPrefix(event string, prefix string) uint64
	ReplaceHandlersByID(es []EventHandler, opt ...bool)
	ReplaceHandlersByEventAndID(es []EventHandler, opt ...bool)
	AddHandlers(es []EventHandler, opt ...bool)
	AddEventHandler(e EventHandler, opt ...bool)
	Trigger(name string, ctx *EventCtx) (uint64, error)
	TriggerCatch(name string, data EventData, ctx *EventCtx, log *logrus.Logger) uint64
}

// Manager manages the event/event chains
type Manager struct {
	eventHandlers  map[string]EventHandlerList // Event handler map
	callstackLimit uint64                      // The max number of callback per event
	allowRecursion bool                        // disabled by default
	log            *logrus.Logger
	mux            *sync.RWMutex
}

func NewEventManager(log *logrus.Logger) *Manager {
	evm := &Manager{
		log: log,
	}
	evm.init()
	return evm
}

// Initalizes the EventManager and must be called befor you use it when not using the constructor
func (m *Manager) init() {
	m.mux = &sync.RWMutex{}
	m.eventHandlers = make(map[string]EventHandlerList)
	if m.callstackLimit < 1 {
		m.callstackLimit = DefaultCallstackLimit
	}
}

// Allows recursive executuion of event handlers debending on the setting
func (m *Manager) AllowRecursion(allow bool) {
	m.mux.Lock()
	m.allowRecursion = allow
	m.mux.Unlock()
}

// Returns all registered event handlers mapped by eventName->Handler
func (m *Manager) RegisteredHandlers() map[string]EventHandlerList {
	m.mux.RLock()
	var meta map[string]EventHandlerList = make(map[string]EventHandlerList)
	for e, ec := range m.eventHandlers {
		meta[e] = EventHandlerList{}
		copy(meta[e], ec[:])
	}
	m.mux.RUnlock()
	return meta
}

// Deletes all registered event handlers
func (m *Manager) DeleteAll() {
	m.mux.Lock()
	m.deleteAll()
	m.mux.Unlock()
}

func (m *Manager) deleteAll() {
	m.eventHandlers = make(map[string]EventHandlerList)
}

// DeleteByEvent deletes event handler by its event name their listening on
func (m *Manager) DeleteByEvent(ei string) {
	m.mux.Lock()
	m.deleteByEvent(ei)
	m.mux.Unlock()
}

func (m *Manager) deleteByEvent(ei string) {
	delete(m.eventHandlers, ei)
}

// Deletes the registered event handlers filtered by event name and ID
func (m *Manager) DeleteByEventAndID(ei string, id string) {
	m.mux.Lock()
	m.deleteByEventAndID(ei, id)
	m.mux.Unlock()
}

func (m *Manager) deleteByEventAndID(ei string, id string) {
	ev, ok := m.eventHandlers[ei]
	if !ok || len(ev) < 1 {
		return
	}
	deleted := 0
	for i, e := range ev {
		j := i - deleted
		if e.ID == id {
			if len(m.eventHandlers[ei]) == 1 {
				delete(m.eventHandlers, ei)
				break
			}
			m.eventHandlers[ei] = append(
				m.eventHandlers[ei][:j],
				m.eventHandlers[ei][j+1:]...,
			)
			deleted++
		}
	}
	m.eventHandlers[ei].Sort()
}

// Deletes all event handlers with the given ID ignoring the event name
func (m *Manager) DeleteByID(id string) {
	m.mux.Lock()
	m.deleteByID(id)
	m.mux.Unlock()
}
func (m *Manager) deleteByID(id string) {
	for ei, ev := range m.eventHandlers {
		deleted := 0
		for i, e := range ev {
			j := i - deleted
			if e.ID == id {
				if len(m.eventHandlers[ei]) == 1 {
					delete(m.eventHandlers, ei)
					deleted++
					break
				}
				m.eventHandlers[ei] = append(
					m.eventHandlers[ei][:j], m.eventHandlers[ei][j+1:]...,
				)
				deleted++
			}
		}
		m.eventHandlers[ei].Sort()
	}
}

// Deletes all event handlers that where the ID starts with the given prefix
func (m *Manager) DeleteByIDPrefix(prefix string) {
	m.mux.Lock()
	for ei, ev := range m.eventHandlers {
		deleted := 0
		for i, e := range ev {
			j := i - deleted
			if strings.HasPrefix(e.ID, prefix) {
				if len(m.eventHandlers[ei]) == 1 {
					delete(m.eventHandlers, ei)
					deleted++
					break
				}
				m.eventHandlers[ei] = append(
					m.eventHandlers[ei][:j], m.eventHandlers[ei][j+1:]...,
				)
				deleted++
			}
		}
		m.eventHandlers[ei].Sort()
	}
	m.mux.Unlock()
}

// Returns the number of event handlers registers with the given ID ignoring the event name
func (m *Manager) CountByID(id string) uint64 {
	m.mux.RLock()
	found := uint64(0)
	for _, ev := range m.eventHandlers {
		for _, e := range ev {
			if e.ID == id {
				found++
			}
		}
	}
	m.mux.RUnlock()
	return found
}

// Returns the number of registered event handlers filterd by ID ignoring the name
func (m *Manager) CountByIDPrefix(prefix string) uint64 {
	m.mux.RLock()
	found := uint64(0)
	for _, ev := range m.eventHandlers {
		for _, e := range ev {
			if strings.HasPrefix(e.ID, prefix) {
				found++
			}
		}
	}
	m.mux.RUnlock()
	return found
}

// Returns the number of registered event handlers filterd by name and ID
func (m *Manager) CountByEventAndID(event string, id string) uint64 {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if _, ok := m.eventHandlers[event]; !ok {
		return 0
	}
	found := uint64(0)
	for _, e := range m.eventHandlers[event] {
		if e.ID == id {
			found++
		}
	}
	return found
}

func (m *Manager) CountByEventAndIDPrefix(event string, prefix string) uint64 {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if _, ok := m.eventHandlers[event]; !ok {
		return 0
	}
	found := uint64(0)
	for _, e := range m.eventHandlers[event] {
		if strings.HasPrefix(e.ID, prefix) {
			found++
		}
	}
	return found
}

func (m *Manager) ReplaceHandlersByEventAndID(es []EventHandler, opt ...bool) {
	m.mux.Lock()
	defer m.mux.Unlock()
	for _, e := range es {
		m.deleteByEventAndID(e.EventName, e.ID)
		m.addHandler(e, opt...)
	}
}
func (m *Manager) ReplaceHandlersByID(es []EventHandler, opt ...bool) {
	m.mux.Lock()
	defer m.mux.Unlock()
	for _, e := range es {
		m.deleteByID(e.ID)
		m.addHandler(e, opt...)
	}
}

func (m *Manager) AddHandlers(es []EventHandler, opt ...bool) {
	m.mux.Lock()
	defer m.mux.Unlock()
	for _, e := range es {
		m.addHandler(e, opt...)
	}
}

func (m *Manager) AddEventHandler(e EventHandler, opt ...bool) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.addHandler(e, opt...)
}

// AddEventHandler adds an event handler to the EventManager
// the provided function must not start goroutines that trigger other events
// that use references of the event or event context!
func (m *Manager) addHandler(e EventHandler, opt ...bool) {
	// !TODO remove "opt" parameter or rename it to prefixCheck (better naming)
	if e.Func == nil {
		return
	}
	_, ok := m.eventHandlers[e.EventName]
	if !ok {
		m.eventHandlers[e.EventName] = []EventHandler{}
	}
	if len(opt) > 0 && opt[0] {
		var prefixCheck bool
		if len(opt) > 1 {
			prefixCheck = opt[1]
		}
		for _, xe := range m.eventHandlers[e.EventName] {
			if prefixCheck {
				if strings.HasPrefix(xe.ID, e.ID) {
					return
				}
			} else {
				if xe.ID == e.ID {
					return
				}
			}
		}
		m.eventHandlers[e.EventName] = append(m.eventHandlers[e.EventName], e)
		m.eventHandlers[e.EventName].Sort()
	} else {
		m.eventHandlers[e.EventName] = append(m.eventHandlers[e.EventName], e)
		m.eventHandlers[e.EventName].Sort()
	}
}

// Triggers an event with the given data and context and logs potential errors
func (m *Manager) TriggerCatch(name string, data EventData, ctx *EventCtx, logger *logrus.Logger) uint64 {
	m.mux.RLock()
	defer m.mux.RUnlock()
	res, err := m.trigger(name, ctx)
	if err != nil && logger != nil {
		logger.WithFields(logrus.Fields{
			"event_name":    name,
			"error_message": err.Error(),
		}).Error("event execution failed")
	}
	return res

}

// Triggers an event with the given data and context
func (m *Manager) Trigger(name string, ctx *EventCtx) (uint64, error) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.trigger(name, ctx)
}

func (m *Manager) trigger(name string, ctx *EventCtx) (uint64, error) {
	if ctx == nil {
		return 0, nil
	}
	el, ok := m.eventHandlers[name]
	if !ok || len(el) < 1 {
		return 0, nil
	}
	ctx.EventName = name
	if err := ctx.addEventSource(name, m.allowRecursion); err != nil {
		// also checks for recursion and stops if not allowed
		return 0, err
	}
	for _, e := range el {
		if ctx.GoContext != nil {
			select {
			case <-ctx.GoContext.Done():
				return ctx.Interations, ctx.GoContext.Err()
			default:
			}
		}
		callerID := fmt.Sprintf("%s.%s", e.EventName, e.ID)
		if m.callstackLimit > 0 && ctx.Interations > m.callstackLimit {
			return ctx.Interations, errors.New("callstack limit exceeded")
		}
		if ctx.StopPropagation {
			return ctx.Interations, nil
		}
		if m.log != nil {
			m.log.WithFields(logrus.Fields{
				"event":    e.EventName,
				"event_id": e.ID,
			}).Debug("executing event handler")
		}
		/*if !m.allowRecursion && ctx.CallStack.Contains(callerID) {
			return ctx.Interations, errors.New("recursion is not allowed")
		}*/
		ctx.pushCallStack(callerID)
		ctx.HandlerID = e.ID
		e.Func(ctx)
		ctx.Interations += 1
	}
	return ctx.Interations, nil
}

type MockManager struct {
	Observer
}

func (m *MockManager) RegisteredHandlers() map[string]EventHandlerList            { return nil }
func (m *MockManager) DeleteAll()                                                 {}
func (m *MockManager) DeleteByEvent(ei string)                                    {}
func (m *MockManager) DeleteByEventAndID(ei string, id string)                    {}
func (m *MockManager) DeleteByID(id string)                                       {}
func (m *MockManager) DeleteByIDPrefix(prefix string)                             {}
func (m *MockManager) CountByID(id string) uint64                                 { return 0 }
func (m *MockManager) CountByIDPrefix(prefix string) uint64                       { return 0 }
func (m *MockManager) CountByEventAndID(event string, id string) uint64           { return 0 }
func (m *MockManager) CountByEventAndIDPrefix(event string, prefix string) uint64 { return 0 }
func (m *MockManager) ReplaceHandlersByID(es []EventHandler, opt ...bool)         {}
func (m *MockManager) ReplaceHandlersByEventAndID(es []EventHandler, opt ...bool) {}
func (m *MockManager) AddHandlers(es []EventHandler, opt ...bool)                 {}
func (m *MockManager) AddEventHandler(e EventHandler, opt ...bool)                {}
func (m *MockManager) Trigger(name string, ctx *EventCtx) (uint64, error) {
	return 0, nil
}
func (m *MockManager) TriggerCatch(name string, ctx *EventCtx, log *logrus.Logger) uint64 {
	return 0
}
