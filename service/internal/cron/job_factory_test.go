package cron

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
)

func TestDefaultJobFactory_RegisterFactory(t *testing.T) {
	mockRegistry := mocks.NewMockCronScheduleRegistry(t)
	factory := NewJobFactory(mockRegistry)
	jobType := IntegrationTestJobType
	var defaultSchedule *core.CronScheduleDefinition

	err := factory.RegisterFactory(nil, jobType, func() (core.CronJob, error) {
		return nil, nil
	}, defaultSchedule)
	require.NoError(t, err)

	// Assert that the factory is registered
	_, ok := factory.factories[jobType]
	assert.True(t, ok, "Factory should be registered")

	// Assert that the default schedule is registered
	_, ok = factory.defaultConfigs[jobType]
	assert.False(t, ok, "Default schedule should not be registered")
}

func TestDefaultJobFactory_RegisterFactory_WithDefaultSchedule(t *testing.T) {
	mockRegistry := mocks.NewMockCronScheduleRegistry(t)
	factory := NewJobFactory(mockRegistry)
	jobType := "core.test.job"
	defaultSchedule := &core.CronScheduleDefinition{
		Type: core.CronScheduleTypeDaily,
	}

	err := factory.RegisterFactory(nil, jobType, func() (core.CronJob, error) {
		return nil, nil
	}, defaultSchedule)
	require.NoError(t, err)

	// Assert that the factory is registered
	_, ok := factory.factories[jobType]
	assert.True(t, ok, "Factory should be registered")

	// Assert that the default schedule is registered
	sched, ok := factory.defaultConfigs[jobType]
	assert.True(t, ok, "Default schedule should be registered")
	assert.Equal(t, defaultSchedule, sched, "Default schedule should be equal")
}

func TestDefaultJobFactory_CreateJob(t *testing.T) {
	mockRegistry := mocks.NewMockCronScheduleRegistry(t)
	factory := NewJobFactory(mockRegistry)
	jobType := "core.test.job"

	err := factory.RegisterFactory(nil, jobType, func() (core.CronJob, error) {
		return mocks.NewMockCronJob(t), nil
	}, nil)
	require.NoError(t, err)

	job, err := factory.CreateJob(nil, jobType)
	assert.NoError(t, err, "CreateJob should not return an error")
	assert.NotNil(t, job, "CreateJob should return a job")
	assert.IsType(t, mocks.NewMockCronJob(t), job, "CreateJob should return a mockCronJob")
}

func TestDefaultJobFactory_CreateJob_UnknownType(t *testing.T) {
	mockRegistry := mocks.NewMockCronScheduleRegistry(t)
	factory := NewJobFactory(mockRegistry)
	jobType := "core.unknown.job"

	job, err := factory.CreateJob(nil, jobType)
	assert.Error(t, err, "CreateJob should return an error")
	assert.Nil(t, job, "CreateJob should return nil")
	assert.Equal(t, fmt.Sprintf("unknown job type: %s", jobType), err.Error(), "Error message should match")
}

func TestDefaultJobFactory_GetDefaultSchedule(t *testing.T) {
	mockRegistry := mocks.NewMockCronScheduleRegistry(t)
	factory := NewJobFactory(mockRegistry)
	jobType := "core.test.job"
	defaultSchedule := &core.CronScheduleDefinition{
		Type: core.CronScheduleTypeDaily,
	}

	err := factory.RegisterFactory(nil, jobType, func() (core.CronJob, error) {
		return nil, nil
	}, defaultSchedule)
	require.NoError(t, err)

	sched, ok := factory.GetDefaultSchedule(nil, jobType)
	assert.True(t, ok, "GetDefaultSchedule should return true")
	assert.Equal(t, defaultSchedule, sched, "GetDefaultSchedule should return the default schedule")
}

func TestDefaultJobFactory_GetDefaultSchedule_NotFound(t *testing.T) {
	mockRegistry := mocks.NewMockCronScheduleRegistry(t)
	factory := NewJobFactory(mockRegistry)
	jobType := "core.unknown.job"

	sched, ok := factory.GetDefaultSchedule(nil, jobType)
	assert.False(t, ok, "GetDefaultSchedule should return false")
	assert.Nil(t, sched, "GetDefaultSchedule should return nil")
}
