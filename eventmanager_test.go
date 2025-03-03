package eventmanager

import (
	"context"
	"testing"
	"time"
)

type Counter struct {
	Value int
}

func TestEnforceIDUniqueness(t *testing.T) {
	evm := NewObserver(nil)
	evm.AddHandlers([]EventHandler{
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
	}, true)
	if count := evm.CountByEventAndID("event_b", "id1"); count != 1 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}

func TestCountByEventAndID(t *testing.T) {
	evm := NewObserver(nil)
	evm.AddHandlers([]EventHandler{
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
	if count := evm.CountByEventAndID("event_a", "id1"); count != 1 {
		t.Errorf("expected count to equal 1 but got %d", count)
	}
	if count := evm.CountByEventAndID("event_b", "id1"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}

func TestCountByEventAndIDPrefix(t *testing.T) {
	evm := NewObserver(nil)
	evm.AddHandlers([]EventHandler{
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
	if count := evm.CountByEventAndIDPrefix("event_b", "id"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}
func TestDeleteByID(t *testing.T) {
	evm := NewObserver(nil)
	evm.AddHandlers([]EventHandler{
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
	if count := evm.DeleteByID("id1"); count != 2 {
		t.Errorf("expected count to equal 2 but got %d", count)
	}
}

func TestRecursionNotAllowed(t *testing.T) {
	evm := NewObserver(nil)
	evm.AllowRecursion(false)

	commonEventHandler := func(ctx *EventCtx) {
		if d, ok := ctx.Data["counter"].(*Counter); ok && d != nil {
			d.Value = d.Value + 1
		}
	}

	testVal := "value"
	evm.AddHandlers([]EventHandler{
		{
			EventName: "event_a",
			ID:        "id1",
			Func:      func(ctx *EventCtx) {},
			Order:     100,
		},
		{
			EventName: "event_a",
			ID:        "id2",
			Func: func(ctx *EventCtx) {
				commonEventHandler(ctx)
				ctx.Data["test"] = testVal
				evm.Trigger("event_b", ctx)
			},
			Order: 200,
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				commonEventHandler(ctx)
				ctx.Data["test"] = testVal + ".event_b"
				evm.Trigger("event_a", ctx)
			},
			Order: 200,
		},
	})

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
		if int(cnt) != counter.Value+1 {
			t.Errorf("invalid number of event handler executions: %d != %d", cnt, counter.Value)
		}
	}

}
func TestRecursionCallLimit(t *testing.T) {
	evm := NewObserver(nil)
	evm.AllowRecursion(true)

	counter := 0
	incr := func() {
		counter += 1
	}
	evm.AddHandlers([]EventHandler{
		{
			EventName: "event_a",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				incr()
			},
			Order: 100,
		},
		{
			EventName: "event_a",
			ID:        "id2",
			Func: func(ctx *EventCtx) {
				incr()
				evm.Trigger("event_b", ctx)
			},
			Order: 200,
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				incr()
				evm.Trigger("event_a", ctx)
			},
			Order: 200,
		},
	})

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
	evm := NewObserver(nil)

	evm.AddHandlers([]EventHandler{
		{
			EventName: "event_a",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				time.Sleep(time.Millisecond * 1100)
			},
			Order: 100,
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
			Order: 100,
		},
	})

	gctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	ectx := NewEventContext(gctx)

	if cnt, err := evm.Trigger("event_a", ectx); err == nil {
		t.Errorf("error expected but got %v. iterations: %d", err, cnt)
	}

	// ensure the second event handler wasn't executed
	if len(ectx.CallStack) > 1 {
		t.Errorf("expected exactly one event handler to be executed but got %d executions", len(ectx.CallStack))
	}

}
