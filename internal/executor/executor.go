package executor

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/device"
	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/observability"
	"github.com/cloud-print/agent/internal/taskqueue"
)

const defaultWorkerCount = 5

type TaskResultCallback func(task *domain.PrintTask)

type Executor struct {
	queue     *taskqueue.Queue
	deviceMgr *device.Manager
	logger    *zap.Logger
	audit     *observability.AuditLogger

	resultMu  sync.RWMutex
	resultCbs []TaskResultCallback

	taskCh chan *domain.PrintTask

	wg      sync.WaitGroup
	cancel  context.CancelFunc
	stopped sync.Once
	closed  bool
	stopMu  sync.Mutex
}

func NewExecutor(queue *taskqueue.Queue, deviceMgr *device.Manager, logger *zap.Logger, audit *observability.AuditLogger) *Executor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Executor{
		queue:     queue,
		deviceMgr: deviceMgr,
		logger:    logger,
		audit:     audit,
	}
}

func (e *Executor) OnTaskResult(callback TaskResultCallback) {
	e.resultMu.Lock()
	defer e.resultMu.Unlock()
	e.resultCbs = append(e.resultCbs, callback)
}

func (e *Executor) fireTaskResult(task *domain.PrintTask) {
	e.resultMu.RLock()
	cbs := make([]TaskResultCallback, len(e.resultCbs))
	copy(cbs, e.resultCbs)
	e.resultMu.RUnlock()

	for _, cb := range cbs {
		cb(task)
	}
}

func (e *Executor) Start(ctx context.Context, workerCount int) {
	if workerCount <= 0 {
		workerCount = defaultWorkerCount
	}

	subCtx, cancel := context.WithCancel(ctx)
	e.stopMu.Lock()
	e.cancel = cancel
	e.closed = false
	e.stopMu.Unlock()

	e.taskCh = make(chan *domain.PrintTask, workerCount)

	for i := 0; i < workerCount; i++ {
		e.wg.Add(1)
		go e.worker(subCtx, i)
	}

	devices := e.deviceMgr.List()
	for _, dev := range devices {
		e.wg.Add(1)
		go e.deviceConsumer(subCtx, dev.DeviceID)
	}

	e.logger.Info("executor started",
		zap.Int("worker_count", workerCount),
		zap.Int("device_count", len(devices)),
	)
}

func (e *Executor) worker(ctx context.Context, id int) {
	defer e.wg.Done()

	for {
		select {
		case <-ctx.Done():
			e.logger.Debug("worker stopped", zap.Int("worker_id", id))
			return
		case task, ok := <-e.taskCh:
			if !ok {
				return
			}
			e.executeTaskLoop(ctx, task)
		}
	}
}

func (e *Executor) deviceConsumer(ctx context.Context, deviceID string) {
	defer e.wg.Done()

	e.logger.Debug("device consumer started",
		zap.String("device_id", deviceID),
	)

	for {
		if ctx.Err() != nil {
			return
		}

		task, err := e.queue.Dequeue(deviceID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.logger.Warn("dequeue failed",
				zap.String("device_id", deviceID),
				zap.Error(err),
			)
			return
		}

		select {
		case <-ctx.Done():
			return
		case e.taskCh <- task:
		}
	}
}

func (e *Executor) Stop() {
	e.stopped.Do(func() {
		e.stopMu.Lock()
		e.closed = true
		cancel := e.cancel
		e.stopMu.Unlock()

		if cancel != nil {
			cancel()
		}
		e.queue.Close()
		e.wg.Wait()
		e.logger.Info("executor stopped")
	})
}
