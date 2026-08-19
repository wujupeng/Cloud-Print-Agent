package taskqueue_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/taskqueue"
)

func newTestQueue(capacity int) *taskqueue.Queue {
	return taskqueue.NewQueue(capacity, nil, zap.NewNop())
}

func makeTask(deviceID, taskID string) *domain.PrintTask {
	return &domain.PrintTask{
		TaskID:   taskID,
		DeviceID: deviceID,
		Status:   domain.TaskStatusPending,
	}
}

func TestFIFOOrder(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	const deviceID = "DEV001"
	for i := 0; i < 5; i++ {
		require.NoError(t, q.Enqueue(makeTask(deviceID, "T"+string(rune('A'+i)))))
	}

	for i := 0; i < 5; i++ {
		task, err := q.TryDequeue(deviceID)
		require.NoError(t, err)
		want := "T" + string(rune('A'+i))
		assert.Equal(t, want, task.TaskID)
	}

	_, err := q.TryDequeue(deviceID)
	require.Error(t, err)
}

func TestFIFOOrder_MixedDevices(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	require.NoError(t, q.Enqueue(makeTask("DEV001", "A1")))
	require.NoError(t, q.Enqueue(makeTask("DEV002", "B1")))
	require.NoError(t, q.Enqueue(makeTask("DEV001", "A2")))
	require.NoError(t, q.Enqueue(makeTask("DEV002", "B2")))
	require.NoError(t, q.Enqueue(makeTask("DEV001", "A3")))

	for i, want := range []string{"A1", "A2", "A3"} {
		task, err := q.TryDequeue("DEV001")
		require.NoError(t, err)
		assert.Equal(t, want, task.TaskID, "DEV001 step %d", i)
	}
	for i, want := range []string{"B1", "B2"} {
		task, err := q.TryDequeue("DEV002")
		require.NoError(t, err)
		assert.Equal(t, want, task.TaskID, "DEV002 step %d", i)
	}
}

func TestQueueCapacity(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	const deviceID = "DEVCAP"
	for i := 0; i < 100; i++ {
		require.NoError(t, q.Enqueue(makeTask(deviceID, "T"+string(rune('A'+i%26))+string(rune('a'+i%26)))))
	}

	err := q.Enqueue(makeTask(deviceID, "OVERFLOW"))
	require.Error(t, err)
	var ae *errs.AgentError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrQueueFull, ae.Code)
	assert.Contains(t, err.Error(), "full")

	assert.Equal(t, 100, q.PendingCount(deviceID))
}

func TestQueueCapacity_PerDevice(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	for i := 0; i < 50; i++ {
		require.NoError(t, q.Enqueue(makeTask("DEVX", "X"+string(rune('a'+i%26))+string(rune('b'+i%26)))))
	}
	for i := 0; i < 100; i++ {
		require.NoError(t, q.Enqueue(makeTask("DEVY", "Y"+string(rune('a'+i%26))+string(rune('b'+i%26)))))
	}

	err := q.Enqueue(makeTask("DEVY", "Y-OVER"))
	require.Error(t, err)
	var ae *errs.AgentError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrQueueFull, ae.Code)

	err = q.Enqueue(makeTask("DEVX", "X-OK"))
	assert.NoError(t, err)
}

func TestCancelTask_Pending(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	const deviceID = "DEVCAN"
	require.NoError(t, q.Enqueue(makeTask(deviceID, "C1")))
	require.NoError(t, q.Enqueue(makeTask(deviceID, "C2")))
	require.NoError(t, q.Enqueue(makeTask(deviceID, "C3")))

	require.NoError(t, q.Cancel(deviceID, "C2"))

	assert.Equal(t, 2, q.PendingCount(deviceID))

	task, err := q.TryDequeue(deviceID)
	require.NoError(t, err)
	assert.Equal(t, "C1", task.TaskID)
	task, err = q.TryDequeue(deviceID)
	require.NoError(t, err)
	assert.Equal(t, "C3", task.TaskID)
}

func TestCancelTask_RunningReturnsError(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	const deviceID = "DEVRUN"
	running := makeTask(deviceID, "R1")
	running.Status = domain.TaskStatusRunning
	require.NoError(t, q.Enqueue(running))

	err := q.Cancel(deviceID, "R1")
	require.Error(t, err)
	var ae *errs.AgentError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrTaskCancelFail, ae.Code)
}

func TestCancelTask_NotFound(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	require.NoError(t, q.Enqueue(makeTask("DEVNF", "N1")))

	err := q.Cancel("DEVNF", "NONEXIST")
	require.Error(t, err)
	var ae *errs.AgentError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrTaskNotFound, ae.Code)

	err = q.Cancel("DEVUNKNOWN", "X")
	require.Error(t, err)
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrTaskNotFound, ae.Code)
}

func TestEnqueue_InvalidTask(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	err := q.Enqueue(nil)
	require.Error(t, err)
	var ae *errs.AgentError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrTaskDataInvalid, ae.Code)

	err = q.Enqueue(&domain.PrintTask{TaskID: "T1"})
	require.Error(t, err)
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrTaskDataInvalid, ae.Code)

	err = q.Enqueue(&domain.PrintTask{DeviceID: "D1"})
	require.Error(t, err)
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrTaskDataInvalid, ae.Code)
}

func TestListPending(t *testing.T) {
	q := newTestQueue(100)
	defer q.Close()

	for i := 0; i < 3; i++ {
		require.NoError(t, q.Enqueue(makeTask("DEVL", "L"+string(rune('a'+i)))))
	}

	list := q.ListPending("DEVL")
	require.Len(t, list, 3)
	assert.Equal(t, "La", list[0].TaskID)
	assert.Equal(t, "Lb", list[1].TaskID)
	assert.Equal(t, "Lc", list[2].TaskID)

	assert.Nil(t, q.ListPending("NOPE"))
}

func TestQueueClose(t *testing.T) {
	q := newTestQueue(100)
	require.NoError(t, q.Enqueue(makeTask("DEVC", "C1")))
	q.Close()

	err := q.Enqueue(makeTask("DEVC", "C2"))
	require.Error(t, err)
}