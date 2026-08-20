package executor

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/protocol"
)

const printSendTimeout = 60 * time.Second

func (e *Executor) executeTaskLoop(ctx context.Context, task *domain.PrintTask) {
	for {
		if ctx.Err() != nil {
			return
		}
		if task.Status.IsTerminal() {
			return
		}

		err := e.executeTask(ctx, task)
		if err == nil {
			return
		}

		if ctx.Err() != nil {
			return
		}

		if isDeviceOfflineErr(err) {
			e.logger.Info("device offline, preserve task for retry",
				zap.String("task_id", task.TaskID),
				zap.String("device_id", task.DeviceID),
			)
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
			continue
		}

		if e.handleRetry(ctx, task, err) {
			continue
		}
		return
	}
}

func (e *Executor) executeTask(ctx context.Context, task *domain.PrintTask) error {
	dev, ok := e.deviceMgr.Get(task.DeviceID)
	if !ok {
		e.markTaskFailed(task, string(errs.ErrDeviceNotFound), "device not found")
		e.fireTaskResult(task)
		return errs.Newf(errs.ErrDeviceNotFound, "device %s not found", task.DeviceID)
	}

	if dev.Status == domain.DeviceStatusOffline || dev.Status == domain.DeviceStatusProbeFailed {
		return errs.Newf(errs.ErrDeviceOffline, "device %s offline", task.DeviceID)
	}

	adapter := protocol.AdapterFor(dev.Protocol)
	if adapter == nil {
		e.markTaskFailed(task, string(errs.ErrProtocolSendFail), "no adapter for protocol "+dev.Protocol.String())
		e.fireTaskResult(task)
		return errs.Newf(errs.ErrProtocolSendFail, "no adapter for protocol %s", dev.Protocol)
	}

	task.Status = domain.TaskStatusRunning
	task.StartedAt = time.Now().UTC()

	if e.audit != nil {
		e.audit.LogTaskExecuted(task.TaskID, task.DeviceID, task.TraceID, task.RetryCount)
	}

	sendCtx, cancel := context.WithTimeout(ctx, printSendTimeout)
	defer cancel()

	data, err := e.loadTaskData(task)
	if err != nil {
		return err
	}
	defer func() {
		if data != nil {
			_ = data.Close()
		}
	}()

	port := dev.DefaultPort()
	if err := adapter.Send(sendCtx, dev.IP, port, data, task.Params); err != nil {
		return err
	}

	task.Status = domain.TaskStatusSuccess
	task.FinishedAt = time.Now().UTC()
	task.ErrorCode = ""
	task.ErrorMsg = ""

	e.logger.Info("task success",
		zap.String("task_id", task.TaskID),
		zap.String("device_id", task.DeviceID),
	)
	e.fireTaskResult(task)
	return nil
}

func (e *Executor) loadTaskData(task *domain.PrintTask) (io.ReadCloser, error) {
	if len(task.Content) > 0 {
		tmpFile, err := os.CreateTemp("", "print-task-*")
		if err != nil {
			return nil, errs.Wrap(errs.ErrStorageIO, "create temp file", err)
		}
		if _, err := tmpFile.Write(task.Content); err != nil {
			tmpFile.Close()
			return nil, errs.Wrap(errs.ErrStorageIO, "write temp file", err)
		}
		tmpFile.Close()
		f, err := os.Open(tmpFile.Name())
		if err != nil {
			return nil, errs.Wrap(errs.ErrStorageIO, "reopen temp file", err)
		}
		return f, nil
	}
	if task.DocumentRef == "" {
		return nil, errs.Newf(errs.ErrTaskDataInvalid, "task %s document_ref empty", task.TaskID)
	}
	f, err := os.Open(task.DocumentRef)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.Newf(errs.ErrTaskNotFound, "task %s data not found", task.TaskID)
		}
		return nil, errs.Wrap(errs.ErrStorageIO, "open task data", err)
	}
	return f, nil
}

func (e *Executor) markTaskFailed(task *domain.PrintTask, code, msg string) {
	task.Status = domain.TaskStatusFailed
	task.FinishedAt = time.Now().UTC()
	task.ErrorCode = code
	task.ErrorMsg = msg
}

func isDeviceOfflineErr(err error) bool {
	var ae *errs.AgentError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Code == errs.ErrDeviceOffline
}
