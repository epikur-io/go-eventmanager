package eventor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type Counter struct {
	Value int
}

func TestEnforceIDUniqueness(t *testing.T) {
	evm := NewObserver()
	err := evm.AddAll([]Handler{
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
		{
			Name: "event_b",
			ID:   "id",
			Func: func(ctx *EventContext) {},
		},
	}, true, false)
	if err == nil {
		t.Errorf("error exepcted but got %v", err)
	}
	if count := evm.CountEventAndID("event_b", "id1"); count != 1 {
		t.Errorf("expected count to equal 1 but got %d", count)
	}

	if count := evm.CountEventAndID("event_b", "id"); count != 1 {
		t.Errorf("expected count to equal 1 but got %d", count)
	}
}

func TestEnforceIDUniquenessWithPrefixCheck(t *testing.T) {
	evm := NewObserver()
	err := evm.AddAll([]Handler{
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
		{
			Name: "event_b",
			ID:   "id",
			Func: func(ctx *EventContext) {},
		},
	}, true, true)
	if err == nil {
		t.Errorf("error exepcted but got %v", err)
	}
	if count := evm.CountEventAndID("event_b", "id1"); count != 1 {
		t.Errorf("expected count to equal 1 but got %d", count)
	}

	if count := evm.CountEventAndID("event_b", "id"); count != 0 {
		t.Errorf("expected count to equal 0 but got %d", count)
	}
}

func TestCountEventAndID(t *testing.T) {
	evm := NewObserver()
	err := evm.AddAll([]Handler{
		{
			Name: "event_a",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
	})
	if err != nil {
		t.Error(err)
	}
	if count := evm.CountEventAndID("event_a", "id1"); count != 1 {
		t.Errorf("expected count to equal 1 but got %d", count)
	}
	if count := evm.CountEventAndID("event_b", "id1"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}

func TestCountEventAndIDPrefix(t *testing.T) {
	evm := NewObserver()
	err := evm.AddAll([]Handler{
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
	})
	if err != nil {
		t.Error(err)
	}
	if count := evm.CountEventAndIDPrefix("event_b", "id"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}
func TestDeleteByID(t *testing.T) {
	evm := NewObserver()
	err := evm.AddAll([]Handler{
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {},
		},
	})

	if err != nil {
		t.Error(err)
	}
	if count := evm.RemoveID("id1"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}

func TestRecursionNotAllowed(t *testing.T) {
	evm := NewObserver()

	commonEventHandler := func(ctx *EventContext) {
		if d, ok := ctx.Data["counter"].(*Counter); ok && d != nil {
			d.Value = d.Value + 1
		}
	}

	callstack := []string{}
	testVal := "value"
	err := evm.AddAll([]Handler{
		{
			Name: "event_a",
			ID:   "id2",
			Func: func(ctx *EventContext) {
				callstack = append(callstack, "event_a.id2")
				commonEventHandler(ctx)
				ctx.Data["test"] = testVal
				evm.DispatchSafe("event_b", ctx)
			},
			Prio: 100,
		},
		{
			Name: "event_a",
			ID:   "id1",
			Func: func(ctx *EventContext) {
				callstack = append(callstack, "event_a.id1")
				commonEventHandler(ctx)
			},
			Prio: 200,
		},
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {
				callstack = append(callstack, "event_b.id1")
				commonEventHandler(ctx)
				ctx.Data["test"] = testVal + ".event_b"
				evm.DispatchSafe("event_a", ctx)
			},
			Prio: 200,
		},
	})
	if err != nil {
		t.Error(err)
	}

	ectx := NewEventContext(context.Background())
	ectx.Data["counter"] = &Counter{}

	if cnt, err := evm.Dispatch("event_a", ectx); err == nil {
		t.Errorf("expected error but got: %v", err)
	} else {
		val, _ := ectx.Data["test"].(string)
		if val != "value.event_b" {
			t.Errorf("expected \"%s\" got \"%s\"", testVal, val)
		}

		counter, _ := ectx.Data["counter"].(*Counter)
		if int(cnt) != counter.Value {
			t.Errorf("invalid number of event handler executions: %d != %d", cnt, counter.Value)
		}
	}
	expectedCallstack := []string{"event_a.id1", "event_a.id2", "event_b.id1"}
	assert.ElementsMatch(t, expectedCallstack, callstack)

}
func TestRecursionCallLimit(t *testing.T) {
	evm := NewObserver(WithRecursionAllowed())

	counter := 0
	incr := func() {
		counter += 1
	}
	err := evm.AddAll([]Handler{
		{
			Name: "event_a",
			ID:   "id1",
			Func: func(ctx *EventContext) {
				incr()
			},
			Prio: 100,
		},
		{
			Name: "event_a",
			ID:   "id2",
			Func: func(ctx *EventContext) {
				incr()
				evm.DispatchSafe("event_b", ctx)
			},
			Prio: 200,
		},
		{
			Name: "event_b",
			ID:   "id1",
			Func: func(ctx *EventContext) {
				incr()
				evm.DispatchSafe("event_a", ctx)
			},
			Prio: 200,
		},
	})
	if err != nil {
		t.Error(err)
	}

	gctx, cancel := context.WithTimeout(context.Background(), time.Second*7)
	defer cancel()
	ectx := NewEventContext(gctx)

	if cnt, err := evm.Dispatch("event_a", ectx); err == nil {
		t.Errorf("error expected but got %v. iterations: %d", err, cnt)
	}

	if counter != 510 {
		t.Errorf("expected counter to equal to 510 but go %d", counter)
	}
}

func TestContextTimeout(t *testing.T) {
	evm := NewObserver()

	err := evm.AddAll([]Handler{
		{
			Name: "event_a",
			ID:   "id1",
			Func: func(ctx *EventContext) {
				time.Sleep(time.Millisecond * 1100)
			},
			Prio: 100,
		},
		{
			Name: "event_a",
			ID:   "id1",
			Func: func(ctx *EventContext) {
				// you can additionally check for expiration inside your own event handlers
				if ctx.Context != nil {
					select {
					case <-ctx.Context.Done():
						return
					default:
					}
				}
			},
			Prio: 100,
		},
	})
	if err != nil {
		t.Error(err)
	}

	gctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	ectx := NewEventContext(gctx)

	if cnt, err := evm.Dispatch("event_a", ectx); err == nil {
		t.Errorf("error expected but got %v. iterations: %d", err, cnt)
	}

	// ensure the second event handler wasn't executed
	if len(ectx.callStack) > 1 {
		t.Errorf("expected exactly one event handler to be executed but got %d executions", len(ectx.callStack))
	}

}

func TestReverseExecutionOrder(t *testing.T) {
	execList := []string{}
	evm := NewObserver(WithExecutionOrder(ExecAscending))
	err := evm.AddAll([]Handler{
		{
			Name: "event_a",
			ID:   "id1",
			Prio: 50,
			Func: func(ctx *EventContext) {
				execList = append(execList, ctx.HandlerID())
			},
		},
		{
			Name: "event_a",
			ID:   "id2",
			Prio: 10,
			Func: func(ctx *EventContext) {
				execList = append(execList, ctx.HandlerID())
			},
		},
	})
	if err != nil {
		t.Error(err)
	}

	gctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	ectx := NewEventContext(gctx)

	if _, err := evm.Dispatch("event_a", ectx); err != nil {
		t.Error(err)
	}

	if len(execList) < 2 {
		t.Error("empty result list, no event handler executed")
	}

	firstEvent := "id2"
	if execList[0] != firstEvent {
		t.Errorf("invalid reverse order, got \"%s\", want \"%s\"", execList[0], firstEvent)
	}

	secondEvent := "id1"
	if execList[1] != secondEvent {
		t.Errorf("invalid reverse order, got \"%s\", want \"%s\"", execList[1], secondEvent)
	}
}

func TestPanicRecover(t *testing.T) {
	expected := any(42)
	result := struct {
		afterCallback bool
		panicValue    any
	}{}
	callback := func(ctx *EventContext, r any) {
		result.panicValue = r
	}
	evm := NewObserver(WithPanicRecover(callback), WithAfterCallback(func(ctx *EventContext) {
		result.afterCallback = true
	}))
	err := evm.AddAll([]Handler{
		{
			Name: "event_a",
			ID:   "id1",
			Prio: 50,
			Func: func(ctx *EventContext) {
				panic(expected)
			},
		},
	})
	if err != nil {
		t.Error(err)
	}

	gctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	ectx := NewEventContext(gctx)

	if _, err := evm.Dispatch("event_a", ectx); err != nil {
		t.Error(err)
	}

	if result.panicValue != expected {
		t.Errorf("invalid value, got \"%s\", want \"%s\"", result.panicValue, expected)
	}

	if !result.afterCallback {
		t.Error("after callback wasn't called")
	}
}

var _ EventDispatcher = &ObserverMock{}

type ObserverMock struct {
	Observer
}

func (m *ObserverMock) RegisteredHandlers() map[string]HandlerList               { return nil }
func (m *ObserverMock) DeleteAll()                                               {}
func (m *ObserverMock) DeleteByEvent(ei string)                                  {}
func (m *ObserverMock) DeleteByEventAndID(ei string, id string)                  {}
func (m *ObserverMock) DeleteByID(id string) uint64                              { return uint64(0) }
func (m *ObserverMock) DeleteByIDPrefix(prefix string) uint64                    { return uint64(0) }
func (m *ObserverMock) CountID(id string) uint64                                 { return 0 }
func (m *ObserverMock) CountIDPrefix(prefix string) uint64                       { return 0 }
func (m *ObserverMock) CountEventAndID(event string, id string) uint64           { return 0 }
func (m *ObserverMock) CountEventAndIDPrefix(event string, prefix string) uint64 { return 0 }
func (m *ObserverMock) AddAll(es []Handler, opt ...bool) error                   { return nil }
func (m *ObserverMock) AddEventHandler(e Handler, opt ...bool) error             { return nil }
func (m *ObserverMock) Dispatch(name string, ctx *EventContext) (uint64, error) {
	return 0, nil
}
func (m *ObserverMock) DispatchSafe(name string, ctx *EventContext) uint64 {
	return 0
}
