package taskqueue

import (
	"sync"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/storage"
)

type deviceQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	tasks  []*domain.PrintTask
	closed bool
}

func newDeviceQueue() *deviceQueue {
	dq := &deviceQueue{}
	dq.cond = sync.NewCond(&dq.mu)
	return dq
}

type Queue struct {
	mu       sync.RWMutex
	queues   map[string]*deviceQueue
	capacity int
	repo     *storage.TaskRepo
	logger   *zap.Logger
	closed   bool
	closeMu  sync.Mutex
}

func NewQueue(capacity int, repo *storage.TaskRepo, logger *zap.Logger) *Queue {
	if capacity <= 0 {
		capacity = 100
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Queue{
		queues:   make(map[string]*deviceQueue),
		capacity: capacity,
		repo:     repo,
		logger:   logger,
	}
}

func (q *Queue) getOrCreate(deviceID string) *deviceQueue {
	q.mu.Lock()
	defer q.mu.Unlock()
	dq, ok := q.queues[deviceID]
	if !ok {
		dq = newDeviceQueue()
		q.queues[deviceID] = dq
	}
	return dq
}

func (q *Queue) get(deviceID string) *deviceQueue {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.queues[deviceID]
}

func (q *Queue) Enqueue(task *domain.PrintTask) error {
	if task == nil {
		return errs.New(errs.ErrTaskDataInvalid, "task is nil")
	}
	if task.DeviceID == "" {
		return errs.New(errs.ErrTaskDataInvalid, "device_id is empty")
	}
	if task.TaskID == "" {
		return errs.New(errs.ErrTaskDataInvalid, "task_id is empty")
	}

	q.closeMu.Lock()
	closed := q.closed
	q.closeMu.Unlock()
	if closed {
		return errs.New(errs.ErrQueueFull, "queue is closed")
	}

	dq := q.getOrCreate(task.DeviceID)

	dq.mu.Lock()
	if dq.closed {
		dq.mu.Unlock()
		return errs.New(errs.ErrQueueFull, "device queue is closed")
	}
	if len(dq.tasks) >= q.capacity {
		dq.mu.Unlock()
		return errs.Newf(errs.ErrQueueFull, "device %s queue full (%d)", task.DeviceID, q.capacity)
	}
	clone := *task
	if clone.Status == "" {
		clone.Status = domain.TaskStatusPending
	}
	if clone.ReceivedAt.IsZero() {
		clone.ReceivedAt = nowUTC()
	}
	dq.tasks = append(dq.tasks, &clone)
	dq.mu.Unlock()

	if q.repo != nil {
		if err := q.repo.SaveTask(task.DeviceID, &clone); err != nil {
			q.logger.Warn("persist task on enqueue failed",
				zap.String("device_id", task.DeviceID),
				zap.String("task_id", task.TaskID),
				zap.Error(err),
			)
		}
	}

	dq.cond.Signal()

	q.logger.Info("task enqueued",
		zap.String("device_id", task.DeviceID),
		zap.String("task_id", task.TaskID),
		zap.Int64("seq_no", clone.SeqNo),
	)
	return nil
}

func (q *Queue) Dequeue(deviceID string) (*domain.PrintTask, error) {
	dq := q.getOrCreate(deviceID)

	dq.mu.Lock()
	defer dq.mu.Unlock()

	for {
		if dq.closed {
			return nil, errs.New(errs.ErrQueueEmpty, "device queue is closed")
		}
		if len(dq.tasks) > 0 {
			task := dq.tasks[0]
			dq.tasks = dq.tasks[1:]
			return task, nil
		}
		dq.cond.Wait()
	}
}

func (q *Queue) TryDequeue(deviceID string) (*domain.PrintTask, error) {
	dq := q.get(deviceID)
	if dq == nil {
		return nil, errs.Newf(errs.ErrQueueEmpty, "device %s queue not found", deviceID)
	}

	dq.mu.Lock()
	defer dq.mu.Unlock()

	if len(dq.tasks) == 0 {
		return nil, errs.Newf(errs.ErrQueueEmpty, "device %s queue empty", deviceID)
	}
	task := dq.tasks[0]
	dq.tasks = dq.tasks[1:]
	return task, nil
}

func (q *Queue) Cancel(deviceID string, taskID string) error {
	dq := q.get(deviceID)
	if dq == nil {
		return errs.Newf(errs.ErrTaskNotFound, "device %s queue not found", deviceID)
	}

	dq.mu.Lock()
	defer dq.mu.Unlock()

	for i, t := range dq.tasks {
		if t.TaskID == taskID {
			if t.Status != domain.TaskStatusPending {
				return errs.Newf(errs.ErrTaskCancelFail,
					"task %s status %s cannot be cancelled", taskID, t.Status)
			}
			dq.tasks = append(dq.tasks[:i], dq.tasks[i+1:]...)

			if q.repo != nil {
				if err := q.repo.DeleteTask(deviceID, taskID); err != nil {
					q.logger.Warn("persist cancel task failed",
						zap.String("device_id", deviceID),
						zap.String("task_id", taskID),
						zap.Error(err),
					)
				}
			}

			q.logger.Info("task cancelled",
				zap.String("device_id", deviceID),
				zap.String("task_id", taskID),
			)
			return nil
		}
	}
	return errs.Newf(errs.ErrTaskNotFound, "task %s not found in device %s", taskID, deviceID)
}

func (q *Queue) PendingCount(deviceID string) int {
	dq := q.get(deviceID)
	if dq == nil {
		return 0
	}
	dq.mu.Lock()
	defer dq.mu.Unlock()
	return len(dq.tasks)
}

func (q *Queue) ListPending(deviceID string) []*domain.PrintTask {
	dq := q.get(deviceID)
	if dq == nil {
		return nil
	}
	dq.mu.Lock()
	defer dq.mu.Unlock()

	result := make([]*domain.PrintTask, len(dq.tasks))
	for i, t := range dq.tasks {
		clone := *t
		result[i] = &clone
	}
	return result
}

func (q *Queue) Close() {
	q.closeMu.Lock()
	q.closed = true
	q.closeMu.Unlock()

	q.mu.Lock()
	queues := make([]*deviceQueue, 0, len(q.queues))
	for _, dq := range q.queues {
		queues = append(queues, dq)
	}
	q.mu.Unlock()

	for _, dq := range queues {
		dq.mu.Lock()
		dq.closed = true
		dq.mu.Unlock()
		dq.cond.Broadcast()
	}
}