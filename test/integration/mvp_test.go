package integration_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/device"
	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/executor"
	"github.com/cloud-print/agent/internal/observability"
	"github.com/cloud-print/agent/internal/taskqueue"
)

func TestMVPPipeline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:9100")
	if err != nil {
		t.Skip("port 9100 unavailable:", err)
	}
	defer ln.Close()

	received := &bytes.Buffer{}
	var receivedMu atomic.Int32
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(received, conn)
				receivedMu.Add(1)
			}(c)
		}
	}()

	logDir := filepath.Join(t.TempDir(), "logs")
	logger, err := observability.NewLogger(logDir, "debug")
	require.NoError(t, err)
	audit, err := observability.NewAuditLogger(logDir)
	require.NoError(t, err)

	devMgr := device.NewManager(logger, nil)
	require.NoError(t, devMgr.Add(&domain.Device{
		DeviceID: "MVPPS01",
		Name:     "mvp-printer",
		IP:       "127.0.0.1",
		Model:    "MOCK-RAW",
		Protocol: domain.ProtocolRAW,
		Status:   domain.DeviceStatusOnline,
		Factory:  "test",
		Port:     9100,
	}))

	queue := taskqueue.NewQueue(100, nil, logger)

	docContent := []byte("MVP-TEST-PRINT-JOB-PAYLOAD\nHello 9100\n")
	docPath := filepath.Join(t.TempDir(), "doc.bin")
	require.NoError(t, os.WriteFile(docPath, docContent, 0o644))

	var (
		resultMu   sync.Mutex
		finalTask  *domain.PrintTask
		gotRunning atomic.Bool
		gotSuccess atomic.Bool
	)

	exec := executor.NewExecutor(queue, devMgr, logger, audit)
	exec.OnTaskResult(func(task *domain.PrintTask) {
		resultMu.Lock()
		defer resultMu.Unlock()
		clone := *task
		finalTask = &clone
		if task.Status == domain.TaskStatusRunning {
			gotRunning.Store(true)
		}
		if task.Status == domain.TaskStatusSuccess {
			gotSuccess.Store(true)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	exec.Start(ctx, 2)
	defer exec.Stop()

	task := &domain.PrintTask{
		TaskID:      "MVP-TASK-001",
		DeviceID:    "MVPPS01",
		DocumentRef: docPath,
		Status:      domain.TaskStatusPending,
		Params:      domain.PrintParams{Copies: 1},
	}
	require.NoError(t, queue.Enqueue(task))

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !gotSuccess.Load() {
		time.Sleep(20 * time.Millisecond)
	}

	require.True(t, gotSuccess.Load(), "task should reach SUCCESS state")

	resultMu.Lock()
	defer resultMu.Unlock()
	require.NotNil(t, finalTask)
	assert.Equal(t, domain.TaskStatusSuccess, finalTask.Status)
	assert.Equal(t, "MVP-TASK-001", finalTask.TaskID)
	assert.Equal(t, "MVPPS01", finalTask.DeviceID)
	assert.False(t, finalTask.StartedAt.IsZero())
	assert.False(t, finalTask.FinishedAt.IsZero())

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && received.Len() < len(docContent) {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, docContent, received.Bytes(), "device should receive exact document bytes")
	assert.GreaterOrEqual(t, receivedMu.Load(), int32(1))
}

func TestMVPPipeline_DeviceOffline(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	logger, err := observability.NewLogger(logDir, "debug")
	require.NoError(t, err)
	audit, err := observability.NewAuditLogger(logDir)
	require.NoError(t, err)

	devMgr := device.NewManager(logger, nil)
	require.NoError(t, devMgr.Add(&domain.Device{
		DeviceID: "MVPOFF01",
		Name:     "offline-printer",
		IP:       "192.0.2.1",
		Model:    "MOCK-OFFLINE",
		Protocol: domain.ProtocolRAW,
		Status:   domain.DeviceStatusOffline,
		Factory:  "test",
		Port:     9100,
	}))

	queue := taskqueue.NewQueue(100, nil, logger)

	docPath := filepath.Join(t.TempDir(), "doc.bin")
	require.NoError(t, os.WriteFile(docPath, []byte("x"), 0o644))

	exec := executor.NewExecutor(queue, devMgr, logger, audit)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exec.Start(ctx, 1)
	defer exec.Stop()

	task := &domain.PrintTask{
		TaskID:      "MVP-OFF-001",
		DeviceID:    "MVPOFF01",
		DocumentRef: docPath,
		Status:      domain.TaskStatusPending,
	}
	require.NoError(t, queue.Enqueue(task))

	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, 0, queue.PendingCount("MVPOFF01"))
}
