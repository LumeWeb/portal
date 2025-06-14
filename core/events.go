package core

import (
	"go.lumeweb.com/event/v2"
	"go.uber.org/zap"
	"reflect"
	"sync"
)

// EventData is an alias for event.M which represents event data as a map
type EventData = event.M

// CoreEvent represents a strongly-typed event in the system.
// It wraps BasicEvent and provides synchronization between the struct fields and event data map.
// P is the type of the payload data structure.
type CoreEvent[P any] struct {
	*event.BasicEvent[EventData]
	Data   P
	mu     *sync.Mutex // protects Data during sync operations
	locked bool        // tracks if we already hold the lock
	logger *Logger
}

// EventHandlerFunc defines a function signature for strongly-typed event handlers.
// P is the type of the event payload.
type EventHandlerFunc[P any] func(*CoreEvent[P]) error

// EventInterceptor is a function that can intercept and modify events before they reach handlers.
// P is the type of the event payload.
type EventInterceptor[P any] func(*CoreEvent[P])

// Set overrides BasicEvent.Set to sync changes back to the struct.
// It sets a value in the event data map and synchronizes it to the struct field.
// key is the field name or tag value to set.
// val is the value to set.
// Returns the event for chaining.
func (e *CoreEvent[P]) Set(key string, val any) event.Event[EventData] {
	if !e.locked {
		e.lock()
		defer e.unlock()
	}
	e.BasicEvent.Set(key, val)
	e.SyncFromMap()
	return e.BasicEvent
}

// SetData overrides BasicEvent.SetData to sync changes.
// It sets the entire event data map and synchronizes it to the struct fields.
// data is the new event data map.
// Returns the event and any error that occurred during setting.
func (e *CoreEvent[P]) SetData(data EventData) (event.Event[EventData], error) {
	if !e.locked {
		e.lock()
		defer e.unlock()
	}
	_, err := e.BasicEvent.SetData(data)
	e.SyncFromMap()
	return e.BasicEvent, err
}

const ValueKey = "_value"

type fieldInfo struct {
	name  string
	index int
}

var (
	typeCache   sync.Map // map[reflect.Type][]fieldInfo
	typeCacheMu sync.Mutex
)

func getCachedFields(t reflect.Type) []fieldInfo {
	if v, ok := typeCache.Load(t); ok {
		return v.([]fieldInfo)
	}

	// Build field info
	fields := make([]fieldInfo, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		key := f.Tag.Get("event")
		if key == "" {
			key = f.Name
		}
		fields = append(fields, fieldInfo{name: key, index: i})
	}

	// Store in cache
	typeCache.Store(t, fields)
	return fields
}

// SyncToMap copies the data from the struct to the underlying EventData map.
// For structs, it uses cached reflection metadata to read fields efficiently.
// For non-struct types, it stores the value under the "_value" key.
func (e *CoreEvent[P]) SyncToMap() {
	if !e.locked {
		e.lock()
		defer e.unlock()
	}
	val := reflect.ValueOf(e.Data)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() == reflect.Struct {
		fields := getCachedFields(val.Type())
		for _, fi := range fields {
			e.Set(fi.name, val.Field(fi.index).Interface())
		}
	} else if val.IsValid() {
		e.Set(ValueKey, val.Interface())
	}
}

// lock acquires the event's mutex and sets the locked flag
func (e *CoreEvent[P]) lock() {
	e.mu.Lock()
	e.locked = true
}

// unlock releases the event's mutex and resets the locked flag
func (e *CoreEvent[P]) unlock() {
	e.locked = false
	e.mu.Unlock()
}

// SyncFromMap copies data from the EventData map back to the struct.
// For structs, it uses reflection to write fields based on map keys and tags.
// For non-struct types, it reads from the "_value" key.
func (e *CoreEvent[P]) SyncFromMap() {
	// Track if we acquired the lock here
	acquiredHere := false
	if !e.locked {
		e.mu.Lock()
		e.locked = true
		acquiredHere = true
	}

	// Ensure we always unlock if we acquired the lock here
	defer func() {
		if acquiredHere {
			e.locked = false
			e.mu.Unlock()
		}
	}()

	// Handle panics in reflection code
	defer func() {
		if r := recover(); r != nil {
			// Log the error using our own logger
			if e.logger != nil {
				e.logger.Error("panic in SyncFromMap", zap.Any("error", r))
			}
			panic(r) // Re-throw the panic after cleanup
		}
	}()

	val := reflect.ValueOf(&e.Data).Elem()
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() == reflect.Struct {
		typ := val.Type()
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			key := field.Tag.Get("event")
			if key == "" {
				key = field.Name
			}
			value := e.BasicEvent.Get(key)
			if value != nil {
				fieldValue := val.Field(i)
				if fieldValue.CanSet() {
					fieldValue.Set(reflect.ValueOf(value))
				}
			}
		}
	} else {
		value := e.BasicEvent.Get(ValueKey)
		if value != nil {
			val.Set(reflect.ValueOf(value))
		}
	}
}

// WithInterceptor wraps a handler with interceptors that will run before the handler.
// handler is the event handler function to wrap.
// interceptors are the interceptor functions to apply.
// Returns a new handler function that runs the interceptors before the original handler.
func WithInterceptor[P any](
	handler EventHandlerFunc[P],
	interceptors ...EventInterceptor[P],
) EventHandlerFunc[P] {
	return func(e *CoreEvent[P]) error {
		for _, interceptor := range interceptors {
			interceptor(e)
		}
		return handler(e)
	}
}

// syncInterceptor is the built-in interceptor that ensures event data is synced
// from the struct to the map before handler execution.
func syncInterceptor[P any](e *CoreEvent[P]) {
	e.SyncToMap()
}

// NewEvent creates a new CoreEvent with the given name and payload data.
// name is the event name.
// data is the payload data structure.
// Returns a new initialized CoreEvent.
func NewEvent[P any](name string, data *P) *CoreEvent[P] {
	return &CoreEvent[P]{
		BasicEvent: event.NewBasic[EventData](name, make(EventData)),
		Data:       *data,
		mu:         &sync.Mutex{},
		logger:     NewLogger(nil, zap.NewNop()), // Initialize with no-op logger by default
	}
}

// SetLogger sets the logger for the event.
func (e *CoreEvent[P]) SetLogger(logger *Logger) {
	e.logger = logger
}

// Fire dispatches an event with the given name and payload using the context's event manager.
// This variant accepts a pointer to the payload data, which allows handlers to modify the original data.
// Use this when:
// - You want handlers to be able to modify the original payload
// - The payload is large and you want to avoid copying
// - The payload is already a pointer (e.g. database models)
//
// ctx is the context containing the event manager.
// eventName is the name of the event to fire.
// data is a pointer to the payload data to send with the event.
// Returns any error that occurred during firing.
func Fire[P any](ctx Context, eventName string, data *P) error {
	err, _ := FireAndReturn(ctx, eventName, data)
	return err
}

// FireAndReturn dispatches an event and returns both the error and event object.
// This is similar to Fire but provides access to the event object for inspection.
// See Fire documentation for guidance on when to use pointer vs value payloads.
func FireAndReturn[P any](ctx Context, eventName string, data *P) (error, event.Event[*CoreEvent[P]]) {
	coreEvent := NewEvent[P](eventName, data)
	coreEvent.SetLogger(ctx.Logger())
	coreEvent.SyncToMap()
	return event.FireTyped[*CoreEvent[P]](ctx.Event(), eventName, coreEvent)
}

// FireByValue dispatches an event with the given name and payload using the context's event manager.
// This variant accepts the payload data by value, creating a copy that handlers cannot modify.
// Use this when:
// - You want to ensure handlers cannot modify the original data
// - The payload is small and copying is negligible
// - You're working with simple value types (numbers, strings, etc.)
//
// ctx is the context containing the event manager.
// eventName is the name of the event to fire.
// data is the payload data to send with the event (passed by value).
// Returns any error that occurred during firing.
func FireByValue[P any](ctx Context, eventName string, data P) error {
	err, _ := FireByValueAndReturn[P](ctx, eventName, data)
	return err
}

// FireByValueAndReturn dispatches an event by value and returns both the error and event object.
// This is similar to FireByValue but provides access to the event object for inspection.
// See FireByValue documentation for guidance on when to use value payloads.
func FireByValueAndReturn[P any](ctx Context, eventName string, data P) (error, event.Event[*CoreEvent[P]]) {
	coreEvent := NewEvent[P](eventName, &data)
	coreEvent.SetLogger(ctx.Logger())
	coreEvent.SyncToMap()
	return event.FireTyped[*CoreEvent[P]](ctx.Event(), eventName, coreEvent)
}

// MustFire dispatches an event and panics if there's an error.
// ctx is the context containing the event manager.
// eventName is the name of the event to fire.
// payload is the data to send with the event.
func MustFire[P any](ctx Context, eventName string, payload *P) {
	if err := Fire[P](ctx, eventName, payload); err != nil {
		panic(err)
	}
}

// FireAsync dispatches an event asynchronously using the context's event manager.
// ctx is the context containing the event manager.
// eventName is the name of the event to fire.
// payload is the data to send with the event (passed by pointer).
func FireAsync[P any](ctx Context, eventName string, payload *P) {
	coreEvent := NewEvent[P](eventName, payload)
	coreEvent.SetLogger(ctx.Logger())
	coreEvent.SyncToMap()
	ctx.Event().Async(eventName, coreEvent)
}

// FireAsyncByValue dispatches an event asynchronously using the context's event manager.
// ctx is the context containing the event manager.
// eventName is the name of the event to fire.
// data is the payload data to send with the event (passed by value).
func FireAsyncByValue[P any](ctx Context, eventName string, data P) {
	coreEvent := NewEvent[P](eventName, &data)
	coreEvent.SetLogger(ctx.Logger())
	coreEvent.SyncToMap()
	ctx.Event().Async(eventName, coreEvent)
}

// Listen registers a strongly-typed event handler using the context's event manager.
// ctx is the context containing the event manager.
// eventName is the name of the event to listen for.
// handler is the function to call when the event occurs.
// priority is an optional priority value for ordering handlers.
func Listen[P any](ctx Context, eventName string, handler EventHandlerFunc[P], priority ...int) {
	// Wrap the handler with our sync interceptor
	wrappedHandler := WithInterceptor(handler, syncInterceptor[P])

	// Create a listener function that matches the event library's expected signature
	listener := event.NewListenerFunc[*CoreEvent[P]](func(e event.Event[*CoreEvent[P]]) error {
		return wrappedHandler(e.Data())
	})

	event.OnTyped[*CoreEvent[P]](
		ctx.Event(),
		eventName,
		listener,
		priority...,
	)
}
