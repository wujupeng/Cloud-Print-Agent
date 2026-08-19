package taskqueue

import (
	"sort"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/storage"
)

type Recoverer struct {
	queue  *Queue
	repo   *storage.TaskRepo
	logger *zap.Logger
}

func NewRecoverer(queue *Queue, repo *storage.TaskRepo, logger *zap.Logger) *Recoverer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Recoverer{queue: queue, repo: repo, logger: logger}
}

func (r *Recoverer) Recover() error {
	if r.repo == nil {
		return nil
	}

	recovery := storage.NewRecovery(r.repo, r.logger)
	result, err := recovery.RecoverAll()
	if err != nil {
		return errs.Wrap(errs.ErrStorageIO, "recover tasks", err)
	}

	for deviceID, tasks := range result {
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].SeqNo < tasks[j].SeqNo
		})

		for _, task := range tasks {
			clone := *task
			clone.Status = domain.TaskStatusPending
			if err := r.enqueueRecovered(&clone); err != nil {
				r.logger.Warn("recover enqueue failed",
					zap.String("device_id", deviceID),
					zap.String("task_id", clone.TaskID),
					zap.Error(err),
				)
				continue
			}
			r.logger.Info("recover task enqueued",
				zap.String("device_id", deviceID),
				zap.String("task_id", clone.TaskID),
				zap.Int64("seq_no", clone.SeqNo),
			)
		}
	}

	r.logger.Info("recover completed",
		zap.Int("device_count", len(result)),
	)
	return nil
}

func (r *Recoverer) enqueueRecovered(task *domain.PrintTask) error {
	dq := r.queue.getOrCreate(task.DeviceID)

	dq.mu.Lock()
	defer dq.mu.Unlock()

	if dq.closed {
		return errs.New(errs.ErrQueueFull, "device queue is closed")
	}
	if len(dq.tasks) >= r.queue.capacity {
		return errs.Newf(errs.ErrQueueFull, "device %s queue full", task.DeviceID)
	}
	dq.tasks = append(dq.tasks, task)
	dq.cond.Signal()
	return nil
}