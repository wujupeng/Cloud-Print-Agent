package taskqueue

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/storage"
)

const tasksDataSubDir = "tasks"

type Persister struct {
	repo   *storage.TaskRepo
	logger *zap.Logger
}

func NewPersister(repo *storage.TaskRepo, logger *zap.Logger) *Persister {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Persister{repo: repo, logger: logger}
}

func (p *Persister) SaveTask(task *domain.PrintTask) error {
	if p.repo == nil {
		return nil
	}
	return p.repo.SaveTask(task.DeviceID, task)
}

func (p *Persister) DeleteTask(deviceID, taskID string) error {
	if p.repo == nil {
		return nil
	}
	return p.repo.DeleteTask(deviceID, taskID)
}

func tasksDataDir(dataDir string) string {
	return filepath.Join(dataDir, tasksDataSubDir)
}

func taskDataPath(taskID, dataDir string) string {
	return filepath.Join(tasksDataDir(dataDir), fmt.Sprintf("%s.bin", taskID))
}

func SaveTaskData(taskID string, data io.Reader, dataDir string) (string, error) {
	if taskID == "" {
		return "", errs.New(errs.ErrTaskDataInvalid, "task_id is empty")
	}
	if dataDir == "" {
		return "", errs.New(errs.ErrConfigInvalid, "data_dir is empty")
	}

	dir := tasksDataDir(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", errs.Wrap(errs.ErrStorageIO, "mkdir tasks data dir", err)
	}

	path := taskDataPath(taskID, dataDir)
	f, err := os.Create(path)
	if err != nil {
		return "", errs.Wrap(errs.ErrStorageIO, "create task data file", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, data); err != nil {
		_ = os.Remove(path)
		return "", errs.Wrap(errs.ErrStorageIO, "write task data", err)
	}
	return path, nil
}

func LoadTaskData(taskID string, dataDir string) (io.ReadCloser, error) {
	if taskID == "" {
		return nil, errs.New(errs.ErrTaskDataInvalid, "task_id is empty")
	}
	path := taskDataPath(taskID, dataDir)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.Newf(errs.ErrTaskNotFound, "task data %s not found", taskID)
		}
		return nil, errs.Wrap(errs.ErrStorageIO, "open task data file", err)
	}
	return f, nil
}

func CleanupTaskData(taskID string, dataDir string) error {
	if taskID == "" {
		return errs.New(errs.ErrTaskDataInvalid, "task_id is empty")
	}
	path := taskDataPath(taskID, dataDir)
	err := os.Remove(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errs.Wrap(errs.ErrStorageIO, "remove task data file", err)
	}
	return nil
}