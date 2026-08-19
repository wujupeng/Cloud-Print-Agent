package storage

import (
	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
)

type Recovery struct {
	repo   *TaskRepo
	logger *zap.Logger
}

func NewRecovery(repo *TaskRepo, logger *zap.Logger) *Recovery {
	return &Recovery{repo: repo, logger: logger}
}

func (r *Recovery) RecoverAll() (map[string][]*domain.PrintTask, error) {
	devices, err := r.repo.ListDeviceBuckets()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]*domain.PrintTask)
	for _, deviceID := range devices {
		tasks, err := r.repo.LoadTasks(deviceID)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if !needsRecover(task.Status) {
				continue
			}
			task.Status = domain.TaskStatusPending
			task.StartedAt = task.ReceivedAt
			if err := r.repo.SaveTask(deviceID, task); err != nil {
				return nil, err
			}
			if r.logger != nil {
				r.logger.Info("recover task",
					zap.String("device_id", deviceID),
					zap.String("task_id", task.TaskID),
					zap.Int64("seq_no", task.SeqNo),
				)
			}
			result[deviceID] = append(result[deviceID], task)
		}
	}
	return result, nil
}

func needsRecover(status domain.TaskStatus) bool {
	return status == domain.TaskStatusRunning || status == domain.TaskStatusRetrying
}