package eventmanager

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
	err := evm.AddHandlers([]EventHandler{
		{
			EventName: "event_b",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
		{
			EventName: "event_b",
			ID:        "id",
			Func:      func(ctx *EventCtx) {},
		},
	}, true, true)
	if err == nil {
		t.Errorf("error exepcted but got %v", err)
	}
	if count := evm.CountByEventAndID("event_b", "id1"); count != 1 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}

func TestCountByEventAndID(t *testing.T) {
	evm := NewObserver()
	err := evm.AddHandlers([]EventHandler{
		{
			EventName: "event_a",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
	})
	if err != nil {
		t.Error(err)
	}
	if count := evm.CountByEventAndID("event_a", "id1"); count != 1 {
		t.Errorf("expected count to equal 1 but got %d", count)
	}
	if count := evm.CountByEventAndID("event_b", "id1"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}

func TestCountByEventAndIDPrefix(t *testing.T) {
	evm := NewObserver()
	err := evm.AddHandlers([]EventHandler{
		{
			EventName: "event_b",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
	})
	if err != nil {
		t.Error(err)
	}
	if count := evm.CountByEventAndIDPrefix("event_b", "id"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}
func TestDeleteByID(t *testing.T) {
	evm := NewObserver()
	err := evm.AddHandlers([]EventHandler{
		{
			EventName: "event_b",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
		},
	})

	if err != nil {
		t.Error(err)
	}
	if count := evm.DeleteByID("id1"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}

func TestRecursionNotAllowed(t *testing.T) {
	evm := NewObserver()

	commonEventHandler := func(ctx *EventCtx) {
		if d, ok := ctx.Data["counter"].(*Counter); ok && d != nil {
			d.Value = d.Value + 1
		}
	}

	callstack := []string{}
	testVal := "value"
	err := evm.AddHandlers([]EventHandler{
		{
			EventName: "event_a",
			ID:        "id2",
			Func: func(ctx *EventCtx) {
				callstack = append(callstack, "event_a.id2")
				commonEventHandler(ctx)
				ctx.Data["test"] = testVal
				evm.TriggerCatch("event_b", ctx)
			},
			Prio: 100,
		},
		{
			EventName: "event_a",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				callstack = append(callstack, "event_a.id1")
				commonEventHandler(ctx)
			},
			Prio: 200,
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				callstack = append(callstack, "event_b.id1")
				commonEventHandler(ctx)
				ctx.Data["test"] = testVal + ".event_b"
				evm.TriggerCatch("event_a", ctx)
			},
			Prio: 200,
		},
	})
	if err != nil {
		t.Error(err)
	}

	ectx := NewEventContext(context.Background())
	ectx.Data["counter"] = &Counter{}

	if cnt, err := evm.Trigger("event_a", ectx); err == nil {
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
	err := evm.AddHandlers([]EventHandler{
		{
			EventName: "event_a",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				incr()
			},
			Prio: 100,
		},
		{
			EventName: "event_a",
			ID:        "id2",
			Func: func(ctx *EventCtx) {
				incr()
				evm.TriggerCatch("event_b", ctx)
			},
			Prio: 200,
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				incr()
				evm.TriggerCatch("event_a", ctx)
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

	if cnt, err := evm.Trigger("event_a", ectx); err == nil {
		t.Errorf("error expected but got %v. iterations: %d", err, cnt)
	}

	if counter != 510 {
		t.Errorf("expected counter to equal to 510 but go %d", counter)
	}
}

func TestContextTimeout(t *testing.T) {
	evm := NewObserver()

	err := evm.AddHandlers([]EventHandler{
		{
			EventName: "event_a",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				time.Sleep(time.Millisecond * 1100)
			},
			Prio: 100,
		},
		{
			EventName: "event_a",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				// you can additionally check for expiration inside your own event handlers
				if ctx.GoContext != nil {
					select {
					case <-ctx.GoContext.Done():
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

	if cnt, err := evm.Trigger("event_a", ectx); err == nil {
		t.Errorf("error expected but got %v. iterations: %d", err, cnt)
	}

	// ensure the second event handler wasn't executed
	if len(ectx.callStack) > 1 {
		t.Errorf("expected exactly one event handler to be executed but got %d executions", len(ectx.callStack))
	}

}
