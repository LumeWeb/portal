package core_tests

import (
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"sync"
	"testing"
)

func TestNewEvent(t *testing.T) {
	type TestData struct {
		Name string
		Age  int
	}

	// Test with empty name
	data := TestData{Name: "Test", Age: 42}
	evt := core.NewEvent("", &data)

	assert.NotNil(t, evt)
	assert.Equal(t, data, evt.Data) // Should match original data
	assert.NotNil(t, evt.BasicEvent)

	// Verify basic functionality by setting/getting a value
	evt.Set("Name", "Updated")
	assert.Equal(t, "Updated", evt.Data.Name) // Check event copy was updated
	assert.Equal(t, "Test", data.Name)        // Original data should remain unchanged

	// Test with name
	evt = core.NewEvent("test.event", &data)
	assert.Equal(t, "test.event", evt.BasicEvent.Name())
	assert.Equal(t, data, evt.Data) // Should match original data
}

func TestCoreEvent_Set(t *testing.T) {
	type TestData struct {
		Name  string `event:"name"`
		Count int
	}

	// Test data
	data := &TestData{
		Name:  "test",
		Count: 5,
	}

	// Create event
	evt := core.NewEvent("", data)

	// Test setting tagged field
	evt.Set("name", "updated")
	assert.Equal(t, "updated", evt.Data.Name)

	// Test setting untagged field
	evt.Set("Count", 10)
	assert.Equal(t, 10, evt.Data.Count)
}

func TestCoreEvent_SetData(t *testing.T) {
	type TestData struct {
		Name  string `event:"name"`
		Count int
	}

	// Test data
	data := &TestData{
		Name:  "test",
		Count: 5,
	}

	// Create event
	evt := core.NewEvent("", data)

	// Test SetData
	newData := core.EventData{
		"name":  "new",
		"Count": 10,
	}

	_, err := evt.SetData(newData)
	assert.NoError(t, err)
	assert.Equal(t, "new", evt.Data.Name)
	assert.Equal(t, 10, evt.Data.Count)
}

func TestCoreEvent_SyncToMap(t *testing.T) {
	type TestData struct {
		Name  string `event:"name"`
		Count int
		Flag  bool
	}

	// Test data
	data := &TestData{
		Name:  "test",
		Count: 5,
		Flag:  true,
	}

	// Create event
	evt := core.NewEvent("", data)

	// Sync to map
	evt.SyncToMap()

	// Verify map values
	assert.Equal(t, "test", evt.Get("name"))
	assert.Equal(t, 5, evt.Get("Count"))
	assert.Equal(t, true, evt.Get("Flag"))
}

func TestCoreEvent_SyncFromMap(t *testing.T) {
	type TestData struct {
		Name  string `event:"name"`
		Count int
		Flag  bool
	}

	// Test data
	data := &TestData{
		Name:  "test",
		Count: 5,
		Flag:  true,
	}

	// Create event
	evt := core.NewEvent("", data)

	// Modify map directly
	evt.Set("name", "updated")
	evt.Set("Count", 10)
	evt.Set("Flag", false)

	// Sync from map
	evt.SyncFromMap()

	// Verify struct values
	assert.Equal(t, "updated", evt.Data.Name)
	assert.Equal(t, 10, evt.Data.Count)
	assert.Equal(t, false, evt.Data.Flag)
}

func TestWithInterceptor(t *testing.T) {
	// Setup interceptor
	var interceptorCalled bool
	interceptor := func(e *core.CoreEvent[string]) {
		interceptorCalled = true
		e.Data = "intercepted"
	}

	// Setup handler
	var handlerCalled bool
	handler := func(e *core.CoreEvent[string]) error {
		handlerCalled = true
		assert.Equal(t, "intercepted", e.Data)
		return nil
	}

	// Wrap handler
	wrapped := core.WithInterceptor(handler, interceptor)

	// Call wrapped handler
	err := wrapped(&core.CoreEvent[string]{Data: "original"})

	// Verify
	assert.NoError(t, err)
	assert.True(t, interceptorCalled)
	assert.True(t, handlerCalled)
}

// withTestListener creates a test listener that uses a wait group and tracks if it was called
// If returnErr is true, the handler will return an error
func withTestListener[P any](t *testing.T, ctx core.Context, eventName string, expectedData P, returnErr bool) (wg *sync.WaitGroup, called *bool) {
	var w sync.WaitGroup
	var handlerCalled bool

	w.Add(1)
	core.Listen[P](ctx, eventName, func(e *core.CoreEvent[P]) error {
		handlerCalled = true
		assert.Equal(t, expectedData, e.Data)
		w.Done()
		if returnErr {
			return assert.AnError
		}
		return nil
	})

	return &w, &handlerCalled
}

func TestFire(t *testing.T) {
	// Create test context
	ctx := coreTesting.NewTestContext(t)

	// Setup listener
	wg, called := withTestListener(t, ctx, "test.event", "test data", false)

	// Fire event
	err := core.FireByValue[string](ctx, "test.event", "test data")
	assert.NoError(t, err)

	// Wait and verify
	wg.Wait()
	assert.True(t, *called)
}

func TestFire_WithStruct(t *testing.T) {
	type TestPayload struct {
		Name  string `event:"name"`
		Count int    `event:"count"`
	}

	// Create test context
	ctx := coreTesting.NewTestContext(t)

	// Setup expected payload
	expected := TestPayload{
		Name:  "struct test",
		Count: 42,
	}

	// Setup listener
	wg, called := withTestListener(t, ctx, "test.struct", expected, false)

	// Fire event with struct payload
	err := core.Fire(ctx, "test.struct", &expected)
	assert.NoError(t, err)

	// Wait and verify
	wg.Wait()
	assert.True(t, *called)
}

func TestMustFire(t *testing.T) {
	// Create test context
	ctx := coreTesting.NewTestContext(t)

	// Test successful case (no error)
	wg, _ := withTestListener(t, ctx, "test.event", "test data", false)
	assert.NotPanics(t, func() {
		core.MustFire(ctx, "test.event", lo.ToPtr("test data")) 
	})
	wg.Wait()

	// Test error case (handler returns error)
	wg, called := withTestListener(t, ctx, "test.error", "test data", true)
	assert.Panics(t, func() {
		core.MustFire(ctx, "test.error", lo.ToPtr("test data"))
	})
	wg.Wait()
	assert.True(t, *called, "handler should have been called")
}

func TestFireAsync(t *testing.T) {
	// Create test context
	ctx := coreTesting.NewTestContext(t)

	// Setup listener
	wg, called := withTestListener(t, ctx, "test.event", "test data", false)

	// Fire event
	core.FireAsync(ctx, "test.event", lo.ToPtr("test data"))

	// Wait and verify
	wg.Wait()
	assert.True(t, *called)
}

func TestListen(t *testing.T) {
	// Create test context
	ctx := coreTesting.NewTestContext(t)

	// Setup handler
	var handlerCalled bool
	var wg sync.WaitGroup
	wg.Add(1)
	handler := func(e *core.CoreEvent[string]) error {
		handlerCalled = true
		assert.Equal(t, "test data", e.Data)
		wg.Done()
		return nil
	}

	// Listen to event
	core.Listen(ctx, "test.event", handler)

	// Fire event
	err := core.Fire(ctx, "test.event", lo.ToPtr("test data"))
	if err != nil {
		t.Error(err)
	}

	// Wait for event to be handled
	wg.Wait()

	// Verify
	assert.True(t, handlerCalled)
}
