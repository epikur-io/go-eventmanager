package eventor

import "sort"

// EventHandler defines a event handler thats listening on one specific event
type EventHandler struct {
	Name string          `json:"name"` // EventName
	Prio int             `json:"prio"`
	Func func(*EventCtx) `json:"-"`
	ID   string          `json:"id"`
}

// EventHandlerList is a list of event handlers that provides a sorting interface
type EventHandlerList []*EventHandler

// SortAscending sorts the list in ascending order (lowest priority first)
func (s EventHandlerList) Sort(order SortOrder) {
	if order == ExecDescending {
		sort.Sort(s)
	} else {
		sort.Sort(ascendingEventHandlerList(s))
	}
}

func (s EventHandlerList) Len() int {
	return len(s)
}

func (s EventHandlerList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Sort by descending handler priority
func (s EventHandlerList) Less(i, j int) bool {
	return (s[i].Prio) > (s[j].Prio)
}

type ascendingEventHandlerList EventHandlerList

func (s ascendingEventHandlerList) Len() int {
	return len(s)
}

func (s ascendingEventHandlerList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less implements sorting in ascending order (lowest priority first)
func (s ascendingEventHandlerList) Less(i, j int) bool {
	return s[i].Prio < s[j].Prio
}
