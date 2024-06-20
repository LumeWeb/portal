package core

import (
	"fmt"
	"github.com/gookit/event"
	"sort"
	"sync"
)

var (
	eventRegistry      = make(map[string]Eventer)
	eventRegistryMutex sync.RWMutex
)

type Eventer interface {
	event.Event
	SetName(name string) Eventer
}

var _ Eventer = (*Event)(nil)

type Event struct {
	// event name
	name string
	// user data.
	data map[string]any
	// target
	target any
	// mark is aborted
	aborted bool
}

// Abort event loop exec
func (e *Event) Abort(abort bool) {
	e.aborted = abort
}

// Fill event data
func (e *Event) Fill(target any, data event.M) *Event {
	if data != nil {
		e.data = data
	}

	e.target = target
	return e
}

// AttachTo add current event to the event manager.
func (e *Event) AttachTo(em event.ManagerFace) {
	em.AddEvent(e)
}

// Get data by index
func (e *Event) Get(key string) any {
	if v, ok := e.data[key]; ok {
		return v
	}

	return nil
}

// Add value by key
func (e *Event) Add(key string, val any) {
	if _, ok := e.data[key]; !ok {
		e.Set(key, val)
	}
}

// Set value by key
func (e *Event) Set(key string, val any) {
	if e.data == nil {
		e.data = make(map[string]any)
	}

	e.data[key] = val
}

// Name get event name
func (e *Event) Name() string {
	return e.name
}

// Data get all data
func (e *Event) Data() map[string]any {
	return e.data
}

// IsAborted check.
func (e *Event) IsAborted() bool {
	return e.aborted
}

// Target get target
func (e *Event) Target() any {
	return e.target
}

// SetName set event name
func (e *Event) SetName(name string) Eventer {
	e.name = name
	return e
}

// SetData set data to the event
func (e *Event) SetData(data event.M) event.Event {
	if data != nil {
		e.data = data
	}
	return e
}

// SetTarget set event target
func (e *Event) SetTarget(target any) *Event {
	e.target = target
	return e
}

func RegisterEvent(id string, event Eventer) {
	eventRegistryMutex.Lock()
	defer eventRegistryMutex.Unlock()

	if _, ok := eventRegistry[id]; ok {
		panic(fmt.Sprintf("event %s already registered", id))
	}

	event.SetName(id)

	eventRegistry[id] = event
}

func GetEvents() []Eventer {
	eventRegistryMutex.RLock()
	defer eventRegistryMutex.RUnlock()

	keys := make([]string, 0, len(eventRegistry))

	for k := range eventRegistry {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	events := make([]Eventer, 0, len(eventRegistry))

	for _, k := range keys {
		events = append(events, eventRegistry[k])
	}

	return events
}
