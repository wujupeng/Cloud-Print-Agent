package observability_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/observability"
)

func readLogLines(t *testing.T, logDir string) []string {
	t.Helper()
	path := filepath.Join(logDir, "agent.log")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var out []string
	for _, l := range lines {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func parseJSON(t *testing.T, line string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(line), &m))
	return m
}

func TestLogger(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")

	logger, err := observability.NewLogger(logDir, "info")
	require.NoError(t, err)

	logger.Info("hello-world",
		observability.FieldEvent("test_event"),
		observability.FieldTraceID("trace-123"),
		observability.FieldDeviceID("DEV001"),
		observability.FieldTaskID("TASK001"),
	)
	require.NoError(t, logger.Sync())

	lines := readLogLines(t, logDir)
	require.NotEmpty(t, lines)
	last := parseJSON(t, lines[len(lines)-1])

	for _, key := range []string{"ts", "level", "caller", "msg"} {
		assert.Contains(t, last, key, "missing fixed field %q", key)
	}
	assert.Equal(t, "info", last["level"])
	assert.Equal(t, "hello-world", last["msg"])
	assert.Equal(t, "test_event", last["event"])
	assert.Equal(t, "trace-123", last["trace_id"])
	assert.Equal(t, "DEV001", last["device_id"])
	assert.Equal(t, "TASK001", last["task_id"])

	caller, ok := last["caller"].(string)
	require.True(t, ok)
	assert.Contains(t, caller, "observability_test")
}

func TestLogger_LevelFilter(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	logger, err := observability.NewLogger(logDir, "warn")
	require.NoError(t, err)

	logger.Info("should-not-appear")
	logger.Warn("should-appear")
	require.NoError(t, logger.Sync())

	lines := readLogLines(t, logDir)
	require.Len(t, lines, 1)
	m := parseJSON(t, lines[0])
	assert.Equal(t, "warn", m["level"])
	assert.Equal(t, "should-appear", m["msg"])
}

func TestTraceID(t *testing.T) {
	ctx := context.Background()

	tid := observability.TraceIDFromCtx(ctx)
	assert.NotEmpty(t, tid)
	assert.Len(t, tid, 36, "UUIDv7 should be 36 chars with hyphens")

	ctx2 := observability.WithTraceID(ctx, "")
	tid2 := observability.TraceIDFromCtx(ctx2)
	assert.NotEmpty(t, tid2)

	ctx3 := observability.WithTraceID(ctx, "fixed-trace-id")
	tid3 := observability.TraceIDFromCtx(ctx3)
	assert.Equal(t, "fixed-trace-id", tid3)

	ctx4 := observability.WithTraceID(ctx3, "new-trace-id")
	tid4 := observability.TraceIDFromCtx(ctx4)
	assert.Equal(t, "new-trace-id", tid4)
}

func TestTraceID_NilCtx(t *testing.T) {
	tid := observability.TraceIDFromCtx(nil)
	assert.NotEmpty(t, tid)
}

func TestTraceID_LoggerFromCtx(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	base, err := observability.NewLogger(logDir, "debug")
	require.NoError(t, err)

	ctx := observability.WithTraceID(context.Background(), "trace-abc")
	l := observability.LoggerFromCtx(ctx, base)
	require.NotNil(t, l)

	l.Info("with-trace")
	require.NoError(t, base.Sync())

	lines := readLogLines(t, logDir)
	require.NotEmpty(t, lines)
	m := parseJSON(t, lines[len(lines)-1])
	assert.Equal(t, "trace-abc", m["trace_id"])
}

func TestNetMetrics(t *testing.T) {
	m := observability.NewNetMetrics()
	assert.Equal(t, domain.NetClassOK, m.GetNetClass())

	m.Update(12, "210.22.123.254", true, true, domain.NetClassOK)

	assert.Equal(t, 12, m.GetDNSLatencyMs())
	assert.Equal(t, "210.22.123.254", m.GetResolvedIP())
	assert.True(t, m.GetGatewayReach())
	assert.True(t, m.GetLocalNetReach())
	assert.Equal(t, domain.NetClassOK, m.GetNetClass())
	assert.False(t, m.GetLastCheckAt().IsZero())

	m.UpdateNetClass(domain.NetClassDNSResolveFail)
	assert.Equal(t, domain.NetClassDNSResolveFail, m.GetNetClass())

	m.UpdateDNS(50, "1.2.3.4")
	assert.Equal(t, 50, m.GetDNSLatencyMs())
	assert.Equal(t, "1.2.3.4", m.GetResolvedIP())

	m.UpdateReach(false, true)
	assert.False(t, m.GetGatewayReach())
	assert.True(t, m.GetLocalNetReach())
}

func TestNetMetrics_Snapshot(t *testing.T) {
	m := observability.NewNetMetrics()
	m.Update(20, "10.0.0.1", true, false, domain.NetClassCloudGatewayUnreachable)

	snap := m.Snapshot()
	assert.Equal(t, 20, snap.DNSLatencyMs)
	assert.Equal(t, "10.0.0.1", snap.ResolvedIP)
	assert.True(t, snap.GatewayReach)
	assert.False(t, snap.LocalNetReach)
	assert.Equal(t, domain.NetClassCloudGatewayUnreachable, snap.NetClass)
	assert.False(t, snap.LastCheckAt.IsZero())
}

func TestNetMetrics_ToNetTopology(t *testing.T) {
	m := observability.NewNetMetrics()
	m.Update(5, "10.0.0.2", true, true, domain.NetClassOK)

	topo := m.ToNetTopology("print.oascii.com")
	assert.Equal(t, "print.oascii.com", topo.Endpoint)
	assert.Equal(t, "10.0.0.2", topo.ResolvedIP)
	assert.Equal(t, 5, topo.DNSLatencyMs)
	assert.True(t, topo.GatewayReach)
	assert.True(t, topo.LocalNetReach)
	assert.Equal(t, domain.NetClassOK, topo.NetClass)
	assert.False(t, topo.LastCheckAt.IsZero())
}

func TestNetMetrics_Concurrent(t *testing.T) {
	m := observability.NewNetMetrics()
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			m.Update(i, "10.0.0.1", true, true, domain.NetClassOK)
			_ = m.GetNetClass()
			_ = m.Snapshot()
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestAuditLogger(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	audit, err := observability.NewAuditLogger(logDir)
	require.NoError(t, err)

	audit.LogTaskReceived("TASK001", "DEV001", "trace-1")
	audit.LogTaskExecuted("TASK001", "DEV001", "trace-1", 0)
	audit.LogTaskRetry("TASK001", "DEV001", "trace-1", 1, "timeout")
	require.NoError(t, audit.Close())

	path := filepath.Join(logDir, "audit.log")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 3)

	for _, line := range lines {
		if line == "" {
			continue
		}
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		assert.Contains(t, m, "ts")
		assert.Contains(t, m, "level")
		assert.Contains(t, m, "msg")
		assert.Contains(t, m, "event")
	}
}

func TestParseLevel(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	for _, lvl := range []string{"debug", "info", "warn", "error", ""} {
		_, err := observability.NewLogger(logDir, lvl)
		require.NoError(t, err, "level %q should be valid", lvl)
	}
}

func TestNetMetrics_LastCheckAtAdvances(t *testing.T) {
	m := observability.NewNetMetrics()
	m.Update(1, "1.1.1.1", true, true, domain.NetClassOK)
	first := m.GetLastCheckAt()
	time.Sleep(2 * time.Millisecond)
	m.UpdateNetClass(domain.NetClassLocalNetFail)
	second := m.GetLastCheckAt()
	assert.True(t, second.After(first))
}