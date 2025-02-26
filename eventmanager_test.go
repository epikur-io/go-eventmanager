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

/*
func TestEventChains(t *testing.T) {

}

func TestAddHandlers(t *testing.T) {
	evm := GetTestEventManager()
	commonEventHandler := func(ctx.Data EventData, ctx *EventCtx) {
		if d, ok := ctx.Data["counter"].(*Counter); ok && d != nil {
			d.Value = d.Value + 1
		}
	}
	evm.AddHandlers([]EventHandler{
		{
			EventName: "event_a",
			ID:        "id1",
			Func:      commonEventHandler,
			Order:     100,
		},
		{
			EventName: "event_a",
			ID:        "id2",
			Func: func(ctx.Data EventData, ctx *EventCtx) {
				commonEventHandler(ctx.Data, ctx)
			},
			Order: -1,
		},
		{
			EventName: "event_b",
			ID:        "id3",
			Func:      commonEventHandler,
		},
		{
			EventName: "event_b",
			ID:        "id4",
			Func:      commonEventHandler,
		},
	})

	ctx.Data := EventData{
		"counter": &Counter{},
	}
	if cnt, err := evm.Trigger("event_a", ctx.Data, NewContext()); err != nil {
		t.Errorf("event handler failed with error: %s", err.Error())
	} else {
		counter, _ := ctx.Data["counter"].(*Counter)
		if int(cnt) != counter.Value {
			t.Errorf("invalid number of event handler executions: %d != %d", cnt, counter.Value)
		}
	}

}
*/

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
				fmt.Println("event_a.id1")
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
				_, err := evm.Trigger("event_b", ctx)
				if err != nil {
					t.Errorf("no error expected but got %s", err)
				}
			},
			Order: 200,
		},
		{
			EventName: "event_b",
			ID:        "id1",
			Func: func(ctx *EventCtx) {
				commonEventHandler(ctx)
				ctx.Data["test"] = testVal + ".event_b"
				_, err := evm.Trigger("event_a", ctx)
				if err == nil {
					t.Errorf("expected error but got %v", err)
				}
			},
			Order: 200,
		},
	})

	gctx, _ := context.WithTimeout(context.Background(), time.Second*3)
	ectx := NewEventContext(gctx)
	ectx.Data["counter"] = &Counter{}
	if cnt, err := evm.Trigger("event_a", ectx); err != nil {
		t.Errorf("event handler failed with error: %s", err.Error())
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
