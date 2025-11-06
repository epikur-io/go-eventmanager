package eventor

import "sort"

// Handler defines a event handler thats listening on one specific event
type Handler struct {
	Name string              `json:"name"` // EventName
	Prio int                 `json:"prio"`
	Func func(*EventContext) `json:"-"`
	ID   string              `json:"id"`
}

// HandlerList is a list of event handlers that provides a sorting interface
type HandlerList []*Handler

// SortAscending sorts the list in ascending order (lowest priority first)
func (s HandlerList) Sort(order SortOrder) {
	if order == ExecDescending {
		sort.Sort(s)
	} else {
		sort.Sort(ascendingHandlerList(s))
	}
}

func (s HandlerList) Len() int {
	return len(s)
}

func (s HandlerList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Sort by descending handler priority
func (s HandlerList) Less(i, j int) bool {
	return (s[i].Prio) > (s[j].Prio)
}

type ascendingHandlerList HandlerList

func (s ascendingHandlerList) Len() int {
	return len(s)
}

func (s ascendingHandlerList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less implements sorting in ascending order (lowest priority first)
func (s ascendingHandlerList) Less(i, j int) bool {
	return s[i].Prio < s[j].Prio
}
