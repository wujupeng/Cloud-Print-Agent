package executor

import (
	"context"

	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const (
	maxRetryCount  = 3
	retryInitDelay = 5 * time.Second
	retryMaxBackoff = 300 * time.Second
)

func (e *Executor) handleRetry(ctx context.Context, task *domain.PrintTask, err error) bool {
	task.RetryCount++

	if task.RetryCount > maxRetryCount {
		code := string(errs.CodeFromErr(err))
		if code == "" {
			code = string(errs.ErrProtocolSendFail)
		}
		e.markTaskFailed(task, code, err.Error())

		e.logger.Error("task finally failed",
			zap.String("task_id", task.TaskID),
			zap.String("device_id", task.DeviceID),
			zap.Int("retry_count", task.RetryCount),
			zap.Error(err),
		)
		if e.audit != nil {
			e.audit.LogTaskRetry(task.TaskID, task.DeviceID, task.TraceID, task.RetryCount, err.Error())
		}
		e.fireTaskResult(task)
		return false
	}

	task.Status = domain.TaskStatusRetrying
	task.ErrorCode = string(errs.CodeFromErr(err))
	task.ErrorMsg = err.Error()

	backoff := retryInitDelay * time.Duration(1<<(task.RetryCount-1))
	if backoff > retryMaxBackoff {
		backoff = retryMaxBackoff
	}
	task.NextRetryAt = time.Now().UTC().Add(backoff)

	e.logger.Warn("task retry scheduled",
		zap.String("task_id", task.TaskID),
		zap.String("device_id", task.DeviceID),
		zap.Int("retry_count", task.RetryCount),
		zap.Duration("backoff", backoff),
		zap.Error(err),
	)
	if e.audit != nil {
		e.audit.LogTaskRetry(task.TaskID, task.DeviceID, task.TraceID, task.RetryCount, err.Error())
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(backoff):
	}

	if ctx.Err() != nil {
		return false
	}

	task.Status = domain.TaskStatusPending
	task.ErrorCode = ""
	task.ErrorMsg = ""
	task.NextRetryAt = time.Time{}
	return true
}
