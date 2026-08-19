package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"strings"

	"go.etcd.io/bbolt"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const taskBucketPrefix = "tasks:"

type TaskRepo struct {
	db *DB
}

func NewTaskRepo(db *DB) *TaskRepo {
	return &TaskRepo{db: db}
}

func taskBucketName(deviceID string) string {
	return taskBucketPrefix + deviceID
}

func taskKey(seqNo int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(seqNo))
	return b
}

func encodeTask(task *domain.PrintTask) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(task); err != nil {
		return nil, errs.Wrap(errs.ErrStorageIO, "encode task", err)
	}
	return buf.Bytes(), nil
}

func decodeTask(raw []byte) (*domain.PrintTask, error) {
	var task domain.PrintTask
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&task); err != nil {
		return nil, errs.Wrap(errs.ErrStorageCorrupt, "decode task", err)
	}
	return &task, nil
}

func (r *TaskRepo) SaveTask(deviceID string, task *domain.PrintTask) error {
	val, err := encodeTask(task)
	if err != nil {
		return err
	}
	return r.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(taskBucketName(deviceID)))
		if err != nil {
			return errs.Wrap(errs.ErrStorageIO, "create task bucket", err)
		}
		return bucket.Put(taskKey(task.SeqNo), val)
	})
}

func (r *TaskRepo) LoadTasks(deviceID string) ([]*domain.PrintTask, error) {
	var tasks []*domain.PrintTask
	err := r.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(taskBucketName(deviceID)))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, v []byte) error {
			t, err := decodeTask(v)
			if err != nil {
				return err
			}
			tasks = append(tasks, t)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *TaskRepo) DeleteTask(deviceID string, taskID string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(taskBucketName(deviceID)))
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			t, err := decodeTask(v)
			if err != nil {
				return err
			}
			if t.TaskID == taskID {
				return bucket.Delete(k)
			}
		}
		return nil
	})
}

func (r *TaskRepo) ListDeviceBuckets() ([]string, error) {
	var devices []string
	err := r.db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
			n := string(name)
			if strings.HasPrefix(n, taskBucketPrefix) {
				devices = append(devices, strings.TrimPrefix(n, taskBucketPrefix))
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return devices, nil
}