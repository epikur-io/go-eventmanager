package eventmanager

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type Counter struct {
	Value int
}

func GetTestEventManager() Observer {
	return NewEventManager(nil)
}

func TestRecursion(t *testing.T) {
	evm := GetTestEventManager()
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
			Func: func(ctx *EventCtx) {
				time.Sleep(time.Second * 2)
			},
			Order: 100,
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

	gctx, _ := context.WithTimeout(context.Background(), time.Second*3)
	ectx := NewEventContext(gctx)
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
	evm := GetTestEventManager()
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

	gctx, _ := context.WithTimeout(context.Background(), time.Second*7)
	ectx := NewEventContext(gctx)

	if cnt, err := evm.Trigger("event_a", ectx); err == nil {
		t.Errorf("error expected but got %v. iterations: %d", err, cnt)
	}

	fmt.Println("counter:", counter)
	if counter != 510 {
		t.Errorf("expected counter to equal to 510 but go %d", counter)
	}

}
