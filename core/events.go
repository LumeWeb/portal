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
}

type Event struct {
	event.BasicEvent
}

func RegisterEvent(id string, event Eventer) {
	eventRegistryMutex.Lock()
	defer eventRegistryMutex.Unlock()

	if _, ok := eventRegistry[id]; ok {
		panic(fmt.Sprintf("event %s already registered", id))
	}

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
