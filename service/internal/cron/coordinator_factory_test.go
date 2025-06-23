package cron

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/testing/mocks"
	"testing"
)

func TestNewCoordinatorFromContext_Standalone(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create mock CronService and CronJobStateMachineRegistry
		mockCronService := mocks.NewMockCronService(tb)
		mockRegistry := mocks.NewMockCronJobStateMachineRegistry(tb)

		// Set expectations
		mockJobFactory := mocks.NewMockCronJobFactory(tb)
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory)

		// Call the function
		coordinator, err := NewCoordinatorFromContext(ctx, mockCronService, mockRegistry)

		// Assert that there is no error
		assert.NoError(t, err)

		// Assert that the coordinator is of type StandaloneCoordinator
		_, ok := coordinator.(*StandaloneCoordinator)
		assert.True(t, ok)
	})
}

/*
func TestNewCoordinatorFromContext_Cluster(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Mock config to return true for ClusterEnabled
		mockConfig := coreTesting.GetMockConfig(ctx)
		mockConfig.On("ClusterEnabled").Return(true)

		// Create mock CronService
		mockCronService := mocks.NewMockCronService(tb)

		// Call the function
		coordinator, err := NewCoordinatorFromContext(ctx, mockCronService)

		// Assert that there is no error
		assert.NoError(t, err)

		// Assert that the coordinator is of type ClusterCoordinator
		_, ok := coordinator.(*ClusterCoordinator)
		assert.True(t, ok)
	})
}
*/

func TestNewStandaloneCoordinatorFromContext(t *testing.T) {
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create mock CronService and CronJobStateMachineRegistry
		mockCronService := mocks.NewMockCronService(tb)
		mockRegistry := mocks.NewMockCronJobStateMachineRegistry(tb)

		// Set expectations
		mockJobFactory := mocks.NewMockCronJobFactory(tb)
		mockCronService.EXPECT().JobFactory().Return(mockJobFactory)

		// Call the function
		coordinator, err := NewStandaloneCoordinatorFromContext(ctx, mockCronService, mockRegistry)
		require.NoError(t, err)

		// Assert that there is no error
		assert.NoError(t, err)

		// Assert that the coordinator is of type StandaloneCoordinator
		_, ok := coordinator.(*StandaloneCoordinator)
		assert.True(t, ok)
	})
}
