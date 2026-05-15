package core_tests

import (
	"context"
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
)

func TestNewEvent(t *testing.T) {
	type TestData struct {
		Name string
		Age  int
	}

	data := TestData{Name: "Test", Age: 42}
	evt := core.NewEvent("", &data, context.Background())

	assert.NotNil(t, evt)
	assert.Equal(t, data, evt.Data)
	assert.NotNil(t, evt.BasicEvent)
}

func TestNewEvent_WithName(t *testing.T) {
	type TestData struct {
		Name string
		Age  int
	}

	data := TestData{Name: "Test", Age: 42}
	evt := core.NewEvent("test.event", &data, context.Background())

	assert.Equal(t, "test.event", evt.BasicEvent.Name())
	assert.Equal(t, data, evt.Data)
}

func TestNewEvent_NilContext(t *testing.T) {
	type TestData struct {
		Name string
	}

	data := TestData{Name: "Test"}
	evt := core.NewEvent("test.event", &data, nil)

	assert.NotNil(t, evt)
	assert.Equal(t, data, evt.Data)
	assert.Nil(t, evt.Context())
}

func TestCoreEvent_Set(t *testing.T) {
	type TestData struct {
		Name  string `event:"name"`
		Count int
	}

	data := &TestData{Name: "test", Count: 5}
	evt := core.NewEvent("", data, context.Background())

	evt.Set("name", "updated")
	assert.Equal(t, "updated", evt.Data.Name)

	evt.Set("Count", 10)
	assert.Equal(t, 10, evt.Data.Count)
}

func TestCoreEvent_SetData(t *testing.T) {
	type TestData struct {
		Name  string `event:"name"`
		Count int
	}

	data := &TestData{Name: "test", Count: 5}
	evt := core.NewEvent("", data, context.Background())

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

	data := &TestData{Name: "test", Count: 5, Flag: true}
	evt := core.NewEvent("", data, context.Background())

	evt.SyncToMap()

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

	data := &TestData{Name: "test", Count: 5, Flag: true}
	evt := core.NewEvent("", data, context.Background())

	evt.Set("name", "updated")
	evt.Set("Count", 10)
	evt.Set("Flag", false)

	evt.SyncFromMap()

	assert.Equal(t, "updated", evt.Data.Name)
	assert.Equal(t, 10, evt.Data.Count)
	assert.Equal(t, false, evt.Data.Flag)
}

func TestCoreEvent_NonStructValue(t *testing.T) {
	data := "hello"
	evt := core.NewEvent("", &data, context.Background())

	evt.SyncToMap()
	assert.Equal(t, "hello", evt.Get(core.ValueKey))

	evt.Set(core.ValueKey, "world")
	evt.SyncFromMap()
	assert.Equal(t, "world", evt.Data)
}

func TestCoreEvent_Context(t *testing.T) {
	type TestData struct {
		Name string
	}

	data := TestData{Name: "test"}
	ctx := context.Background()
	evt := core.NewEvent("test.event", &data, ctx)

	assert.Equal(t, ctx, evt.Context())

	newCtx := context.WithValue(ctx, "key", "value")
	evt.SetContext(newCtx)
	assert.Equal(t, newCtx, evt.Context())
	assert.Equal(t, "value", evt.Context().Value("key"))
}

func TestWithInterceptor(t *testing.T) {
	var interceptorCalled bool
	interceptor := func(e *core.CoreEvent[string]) {
		interceptorCalled = true
		e.Data = "intercepted"
	}

	var handlerCalled bool
	handler := func(e *core.CoreEvent[string]) error {
		handlerCalled = true
		assert.Equal(t, "intercepted", e.Data)
		return nil
	}

	wrapped := core.WithInterceptor(handler, interceptor)

	err := wrapped(&core.CoreEvent[string]{Data: "original"})

	assert.NoError(t, err)
	assert.True(t, interceptorCalled)
	assert.True(t, handlerCalled)
}

func TestWithInterceptor_Multiple(t *testing.T) {
	var order []string
	interceptor1 := func(e *core.CoreEvent[string]) {
		order = append(order, "first")
	}
	interceptor2 := func(e *core.CoreEvent[string]) {
		order = append(order, "second")
	}

	handler := func(e *core.CoreEvent[string]) error {
		order = append(order, "handler")
		return nil
	}

	wrapped := core.WithInterceptor(handler, interceptor1, interceptor2)
	err := wrapped(&core.CoreEvent[string]{Data: "test"})

	assert.NoError(t, err)
	assert.Equal(t, []string{"first", "second", "handler"}, order)
}

// withTestListener creates a test listener that uses a wait group and tracks if it was called
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
	ctx, err := coreTesting.NewTestContext(t)
	assert.NoError(t, err)

	wg, called := withTestListener(t, ctx, "test.event", "test data", false)

	err = core.FireByValue[string](ctx, "test.event", "test data")
	assert.NoError(t, err)

	wg.Wait()
	assert.True(t, *called)
}

func TestFire_WithStruct(t *testing.T) {
	type TestPayload struct {
		Name  string `event:"name"`
		Count int    `event:"count"`
	}

	ctx, err := coreTesting.NewTestContext(t)
	assert.NoError(t, err)

	expected := TestPayload{Name: "struct test", Count: 42}

	wg, called := withTestListener(t, ctx, "test.struct", expected, false)

	err = core.Fire(ctx, "test.struct", &expected)
	assert.NoError(t, err)

	wg.Wait()
	assert.True(t, *called)
}

func TestMustFire(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	assert.NoError(t, err)

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
	ctx, err := coreTesting.NewTestContext(t)
	assert.NoError(t, err)

	wg, called := withTestListener(t, ctx, "test.event", "test data", false)

	core.FireAsync(ctx, "test.event", lo.ToPtr("test data"))

	wg.Wait()
	assert.True(t, *called)
}

func TestListen(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	assert.NoError(t, err)

	var handlerCalled bool
	var wg sync.WaitGroup
	wg.Add(1)
	handler := func(e *core.CoreEvent[string]) error {
		handlerCalled = true
		assert.Equal(t, "test data", e.Data)
		wg.Done()
		return nil
	}

	core.Listen(ctx, "test.event", handler)

	err = core.Fire(ctx, "test.event", lo.ToPtr("test data"))
	if err != nil {
		t.Error(err)
	}

	wg.Wait()

	assert.True(t, handlerCalled)
}
