package cron

import (
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sync"
	"time"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
)

var _ core.CronMonitor = (*DefaultCronMonitor)(nil)

type DefaultCronMonitor struct {
	ctx           core.Context
	db            *gorm.DB
	logger        *core.Logger
	stopChan      chan struct{}
	maintenanceCh chan struct{}
	cron          core.CronService
	heartbeats    map[uuid.UUID]chan struct{} // Track active heartbeats
	mu            sync.Mutex                  // Protect concurrent access to heartbeats
}

func NewDefaultCronMonitor(ctx core.Context, cron core.CronService) *DefaultCronMonitor {
	return &DefaultCronMonitor{
		ctx:           ctx,
		db:            ctx.DB(),
		logger:        ctx.Logger(),
		cron:          cron,
		stopChan:      make(chan struct{}),
		maintenanceCh: make(chan struct{}, 1),
		heartbeats:    make(map[uuid.UUID]chan struct{}),
	}
}

func (m *DefaultCronMonitor) CleanupOrphanedJobs() (int, error) {
	var count int
	var jobs []models.CronJob
	var toRemove []uuid.UUID

	err := m.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("origin = ?", core.JobOriginPlugin).Find(&jobs).Error; err != nil {
			return err
		}

		for _, job := range jobs {
			if !core.PluginExists(job.SourceID) {
				m.logger.Info("Removing orphaned job",
					zap.String("jobID", job.UUID.String()),
					zap.String("plugin", job.SourceID))

				if err := tx.Delete(&job).Error; err != nil {
					return err
				}
				toRemove = append(toRemove, job.UUID.ToUUID())
				count++
			}
		}
		return nil
	})

	if err == nil {
		// Clean up state machines after successful transaction
		for _, jobID := range toRemove {
			m.cron.StateMachine().RemoveStateMachine(jobID)
		}
	}

	return count, err
}

func (m *DefaultCronMonitor) RequeueStuckJobs() error {
	// Get all jobs that appear dead based on database state
	var potentialDeadJobs []models.CronJob
	heartbeatCutoff := time.Now().Add(-5 * time.Minute)

	query := &models.CronJob{
		State: models.CronJobStateRunning,
	}

	err := m.db.Model(query).Where(query).
		Where("last_heartbeat < ?", heartbeatCutoff).
		Find(&potentialDeadJobs).Error
	if err != nil {
		return fmt.Errorf("failed to query potential dead jobs: %w", err)
	}

	for _, job := range potentialDeadJobs {
		jobID := job.UUID.ToUUID()

		// The database is our source of truth - if it says the job is dead, we treat it as dead
		// Get job instance for logging
		jobInstance, err := m.cron.JobFactory().CreateJob(job.JobType)
		if err != nil {
			m.logger.Error("Failed to create job instance for logging",
				zap.String("jobID", jobID.String()),
				zap.String("jobType", job.JobType),
				zap.Error(err))
		}

		logFields := []zap.Field{
			zap.String("jobType", job.JobType),
			zap.Time("lastHeartbeat", *job.LastHeartbeat),
		}

		if jobInstance != nil {
			logFields = append(logFields, core.JobLogField(jobInstance))
		} else {
			logFields = append(logFields, zap.String("jobID", jobID.String()))
		}

		m.logger.Warn("Detected dead job from database, requeuing", logFields...)

		if err := m.cron.Coordinator().HandleFailedJob(jobID, job.Failures+1); err != nil {
			m.logger.Error("Failed to handle dead job",
				zap.String("jobID", jobID.String()),
				zap.Error(err))
		}
	}

	return nil
}

func (m *DefaultCronMonitor) CleanupCompletedJobs() error {
	err := m.db.
		Where(&models.CronJob{
			State:        models.CronJobStateCompleted,
			ScheduleType: string(core.CronScheduleTypeOnce),
		}).
		Where("created_at < ?", time.Now().Add(-30*24*time.Hour)).
		Delete(&models.CronJob{}).
		Error

	if err != nil {
		return fmt.Errorf("failed to cleanup once jobs: %w", err)
	}
	return nil
}

func (m *DefaultCronMonitor) StartMonitoring() error {
	m.stopChan = make(chan struct{})
	go m.maintenanceLoop()
	return nil
}

func (m *DefaultCronMonitor) StartHeartbeat(jobID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.heartbeats[jobID]; exists {
		m.logger.Warn("Heartbeat already running for job", zap.String("jobID", jobID.String()))
		return
	}

	stopChan := make(chan struct{})
	m.heartbeats[jobID] = stopChan

	go m.heartbeatLoop(jobID, stopChan)
	m.logger.Debug("Started heartbeat for job", zap.String("jobID", jobID.String()))
}

func (m *DefaultCronMonitor) StopHeartbeat(jobID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stopChan, exists := m.heartbeats[jobID]
	if !exists {
		m.logger.Debug("No heartbeat running for job", zap.String("jobID", jobID.String()))
		return
	}

	close(stopChan)
	delete(m.heartbeats, jobID)
	m.logger.Debug("Stopped heartbeat for job", zap.String("jobID", jobID.String()))
}

func (m *DefaultCronMonitor) CheckHeartbeat(jobID uuid.UUID) (bool, error) {
	// Always check coordinator first
	alive, err := m.cron.Coordinator().CheckHeartbeat(jobID)
	if err != nil {
		return false, err
	}

	// If coordinator says dead, ensure we clean up
	if !alive {
		m.mu.Lock()
		delete(m.heartbeats, jobID)
		m.mu.Unlock()
		return false, nil
	}

	// Otherwise return coordinator's status
	return alive, nil
}

func (m *DefaultCronMonitor) heartbeatLoop(jobID uuid.UUID, stopChan <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.cron.Coordinator().SetHeartbeat(jobID); err != nil {
				m.logger.Error("Failed to send heartbeat",
					zap.String("jobID", jobID.String()),
					zap.Error(err))
			}
		case <-stopChan:
			return
		}
	}
}

func (m *DefaultCronMonitor) StopMonitoring() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close the main stop channel first
	if m.stopChan != nil {
		close(m.stopChan)
	}

	// Stop all active heartbeats
	for jobID, stopChan := range m.heartbeats {
		close(stopChan)
		delete(m.heartbeats, jobID)
		m.logger.Debug("Stopped heartbeat during shutdown",
			zap.String("jobID", jobID.String()))
	}

	return nil
}

func (m *DefaultCronMonitor) performMaintenance() error {
	if _, err := m.CleanupOrphanedJobs(); err != nil {
		return fmt.Errorf("failed to cleanup orphaned jobs: %w", err)
	}
	if err := m.RequeueStuckJobs(); err != nil {
		return fmt.Errorf("failed to requeue stuck jobs: %w", err)
	}
	return nil
}

func (m *DefaultCronMonitor) maintenanceLoop() {
	for {
		select {
		case <-m.stopChan:
			m.logger.Info("Stopping maintenance loop")
			return
		case <-m.maintenanceCh: // Listen for signals
			if err := m.performMaintenance(); err != nil {
				m.logger.Error("Failed to perform maintenance (signal)", zap.Error(err))
			}
		}
	}
}

// SignalMaintenance sends a signal to the maintenance loop
func (m *DefaultCronMonitor) SignalMaintenance() {
	select {
	case m.maintenanceCh <- struct{}{}: // Send signal (non-blocking)
	default:
		// Another signal is already pending, or the channel is full.
		// Avoid blocking.
	}
}
