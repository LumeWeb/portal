package service_tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.lumeweb.com/portal/service"
	"go.uber.org/zap"
)

var (
	integrationTestJobSourceId       = "cron.integration-test-job"
	integrationTestPluginJobSourceId = "test"
	integrationTestJobType           = fmt.Sprintf("core.%s", integrationTestJobSourceId)
	integrationTestPluginJobId       = "job1"
	integrationTestPluginName        = "test"
)

// simpleTestJob is a simple job implementation for testing purposes.
type simpleTestJob struct {
	*core.BaseCronJob
	runCallback func()
	mu          sync.Mutex
}

func newSimpleTestJob(origin string, sourceId string, schedule *core.CronScheduleDefinition, runCallback func()) *simpleTestJob {
	id := uuid.New()
	return &simpleTestJob{
		BaseCronJob: core.NewBaseCronJob(
			id,
			origin,
			sourceId,
			"Integration Test Job",
			schedule,
			map[string]interface{}{"test": "value"},
		),
		runCallback: runCallback,
	}
}

func (j *simpleTestJob) Run(ctx core.Context, eventCtx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.runCallback != nil {
		j.runCallback()
	}
	ctx.Logger().Info("Simple test job ran", zap.String("jobID", j.ID().String()))
	return nil
}

// TestCronServiceDefault_RegisterJob_Integration tests the RegisterJob function
// by creating a job, registering it, and verifying that it's stored in the database.
func TestCronServiceDefault_RegisterJob_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		cronService := core.GetService[core.CronService](ctx, core.CRON_SERVICE)
		// Create a simple test job
		testJob := newSimpleTestJob(core.JobOriginCore, integrationTestJobSourceId, &core.CronScheduleDefinition{Type: core.CronScheduleTypeDaily}, nil)

		// Register the job
		err := cronService.RegisterJob(context.Background(), testJob, nil)
		require.NoError(t, err)

		// Verify that the job was created in the database
		var job models.CronJob
		result := db.First(&job, "uuid = ?", types.FromUUID(testJob.ID()))
		require.NoError(t, result.Error)
		assert.Equal(t, "core.cron.integration-test-job", job.JobType)
		assert.Equal(t, core.JobOriginCore, job.Origin)
		assert.Equal(t, "cron.integration-test-job", job.SourceID)

		// Clean up
		err = cronService.Stop(nil)
		require.NoError(t, err)
	}, coreTesting.WithServiceFactory(core.CRON_SERVICE, service.NewCronService))
}

// TestCronServiceDefault_RunJob_Integration tests the RunJob function
// by creating a job, registering it, running it, and verifying that it's executed.
func TestCronServiceDefault_RunJob_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		cronService := core.GetService[core.CronService](ctx, core.CRON_SERVICE)
		defer func() {
			err := cronService.Stop(nil)
			require.NoError(t, err)
		}()

		var jobRan bool

		// Register the job
		err := cronService.RegisterJobType(nil, integrationTestJobType, func() (core.CronJob, error) {
			return newSimpleTestJob(core.JobOriginCore, integrationTestJobSourceId, &core.CronScheduleDefinition{
				Type:   core.CronScheduleTypeOnce,
				AtTime: time.Now().Add(time.Second * 1),
			}, func() {
				jobRan = true
			}), nil
		}, nil)

		require.NoError(t, err)

		job, err := cronService.JobFactory().CreateJob(nil, integrationTestJobType)
		require.NoError(t, err)

		err = cronService.RegisterJob(nil, job, nil)
		require.NoError(t, err)

		// Start the cron service
		err = cronService.Start(nil)
		require.NoError(t, err)

		cronJob, found, err := cronService.GetActiveJob(nil, job.ID())
		require.NoError(t, err)
		require.True(t, found)

		coreJob := cronJob.Job()
		require.NotNil(t, coreJob)

		<-cronJob.Done()

		// Assert that the job has run
		assert.True(t, jobRan, "Job should have run")
	}, coreTesting.WithServiceFactory(core.CRON_SERVICE, service.NewCronService))
}

// TestCronServiceDefault_RegisterPluginJobs_Integration tests the registerPluginJobs function
// by registering plugin jobs and verifying that they are registered correctly.
func TestCronServiceDefault_RegisterPluginJobs_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		cronService := core.GetService[core.CronService](ctx, core.CRON_SERVICE)
		defer func() {
			err := cronService.Stop(nil)
			require.NoError(t, err)
		}()

		// Define a plugin info with cron jobs
		pluginInfo := core.PluginInfo{
			ID: integrationTestPluginName,
			CronJobs: []core.PluginCronJob{
				{
					Name: integrationTestPluginJobId,
					Factory: func() (core.CronJob, error) {
						return newSimpleTestJob(core.JobOriginPlugin, integrationTestPluginJobSourceId, &core.CronScheduleDefinition{
							Type: core.CronScheduleTypeDaily,
						}, nil), nil
					},
					Schedule: &core.CronScheduleDefinition{
						Type: core.CronScheduleTypeDaily,
					},
				},
			},
		}

		core.RegisterPlugin(pluginInfo)

		// Register the plugin jobs
		err := cronService.RegisterPluginJobs(nil, pluginInfo)
		require.NoError(t, err)

		// Verify that the job type is registered
		jobFactory := cronService.JobFactory()
		_, err = jobFactory.CreateJob(nil, "plugin.test.job1")
		require.NoError(t, err)
	}, coreTesting.WithServiceFactory(core.CRON_SERVICE, service.NewCronService))
}

// TestCronServiceDefault_ScheduleRegistry_Integration tests the ScheduleRegistry function
// by registering a schedule and verifying that it is registered correctly.
func TestCronServiceDefault_ScheduleRegistry_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		cronService := core.GetService[core.CronService](ctx, core.CRON_SERVICE)
		defer func() {
			err := cronService.Stop(nil)
			require.NoError(t, err)
		}()

		// Get the schedule registry
		registry := cronService.ScheduleRegistry()
		assert.NotNil(t, registry)

		// Register a new schedule type
		scheduleType := core.CronScheduleType("test_schedule")
		cronService.ScheduleRegistry().Register(scheduleType, func(def core.CronScheduleDefinition) (gocron.JobDefinition, error) {
			return gocron.DurationJob(1 * time.Minute), nil
		})

		// Verify that the schedule type is registered
		registeredTypes := cronService.ScheduleRegistry().GetRegisteredTypes()
		assert.Contains(t, registeredTypes, scheduleType)
	}, coreTesting.WithServiceFactory(core.CRON_SERVICE, service.NewCronService))
}

// TestCronServiceDefault_JobFactory_Integration tests the JobFactory function
// by registering a job type and verifying that it is registered correctly.
func TestCronServiceDefault_JobFactory_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		cronService := core.GetService[core.CronService](ctx, core.CRON_SERVICE)
		defer func() {
			err := cronService.Stop(nil)
			require.NoError(t, err)
		}()

		// Get the job factory
		factory := cronService.JobFactory()
		assert.NotNil(t, factory)

		// Register a new job type
		jobType := integrationTestJobType
		err := cronService.RegisterJobType(nil, integrationTestJobType, func() (core.CronJob, error) {
			return newSimpleTestJob(core.JobOriginCore, integrationTestJobSourceId, &core.CronScheduleDefinition{
				Type: core.CronScheduleTypeDaily,
			}, nil), nil
		}, &core.CronScheduleDefinition{
			Type: core.CronScheduleTypeDaily,
		})
		require.NoError(t, err)

		// Verify that the job type is registered
		_, err = factory.CreateJob(nil, jobType)
		require.NoError(t, err)
	}, coreTesting.WithServiceFactory(core.CRON_SERVICE, service.NewCronService))
}

// TestCronServiceDefault_StateMachine_Integration tests the StateMachine function
// by transitioning a job through different states and verifying that the state is updated correctly.
func TestCronServiceDefault_StateMachine_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		db := ctx.DB()
		require.NotNil(tb, db)

		cronService := core.GetService[core.CronService](ctx, core.CRON_SERVICE)
		defer func() {
			err := cronService.Stop(nil)
			require.NoError(t, err)
		}()

		// Create a simple test job
		testJob := newSimpleTestJob(core.JobOriginCore, integrationTestJobSourceId, &core.CronScheduleDefinition{Type: core.CronScheduleTypeDaily}, nil)

		// Register the job
		err := cronService.RegisterJob(nil, testJob, nil)
		require.NoError(t, err)

		// Get the state machine
		stateMachine := cronService.StateMachine()
		assert.NotNil(t, stateMachine)

		// Transition the job to the running state
		err = cronService.StateMachine().Transition(context.Background(), testJob.ID(), models.CronJobStateRunning)
		require.NoError(t, err)

		// Verify that the job state is updated in the database
		var job models.CronJob
		result := db.First(&job, "uuid = ?", types.FromUUID(testJob.ID()))
		require.NoError(t, result.Error)
		assert.Equal(t, models.CronJobStateRunning, job.State)
	}, coreTesting.WithServiceFactory(core.CRON_SERVICE, service.NewCronService))
}
