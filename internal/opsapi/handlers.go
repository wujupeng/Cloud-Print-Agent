package opsapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cloud-print/agent/internal/domain"
)

const contentTypeJSON = "application/json; charset=utf-8"

func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	healthy := s.isHealthy()
	if !healthy {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) isHealthy() bool {
	if s.netMetrics == nil {
		return true
	}
	return s.netMetrics.GetNetClass() != domain.NetClassLocalNetFail
}

type StatusSummary struct {
	Status        string                 `json:"status"`
	Version       string                 `json:"version"`
	ProcessUptime string                 `json:"process_uptime"`
	CloudEndpoint string                 `json:"cloud_endpoint"`
	DerivedURL    string                 `json:"derived_url"`
	NetClass      domain.NetClass        `json:"net_class"`
	DeviceCount   int                    `json:"device_count"`
	OnlineDevices int                    `json:"online_devices"`
	QueueCount    int                    `json:"queue_count"`
	PendingTasks  int                    `json:"pending_tasks"`
	NetMetrics    interface{}            `json:"net_metrics"`
	GeneratedAt   time.Time              `json:"generated_at"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	devices := s.deviceMgr.List()
	online := 0
	pending := 0
	for _, d := range devices {
		if d.Status == domain.DeviceStatusOnline {
			online++
		}
		pending += s.queue.PendingCount(d.DeviceID)
	}

	summary := StatusSummary{
		Status:        "running",
		Version:       domain.Version,
		ProcessUptime: time.Since(processStart).String(),
		CloudEndpoint: s.cfg.Cloud.Endpoint,
		DerivedURL:    s.cfg.Cloud.DerivedURL,
		DeviceCount:   len(devices),
		OnlineDevices: online,
		QueueCount:    len(devices),
		PendingTasks:  pending,
		GeneratedAt:   time.Now().UTC(),
	}
	if s.netMetrics != nil {
		summary.NetClass = s.netMetrics.GetNetClass()
		summary.NetMetrics = s.netMetrics.Snapshot()
	}
	s.writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	if s.netMetrics == nil {
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "no_metrics"})
		return
	}
	topo := s.netMetrics.ToNetTopology(s.cfg.Cloud.Endpoint)
	s.writeJSON(w, http.StatusOK, topo)
}

type DeviceView struct {
	DeviceID    string             `json:"device_id"`
	Name        string             `json:"name"`
	IP          string             `json:"ip"`
	Protocol    domain.Protocol    `json:"protocol"`
	Status      domain.DeviceStatus `json:"status"`
	LastProbeAt time.Time          `json:"last_probe_at,omitempty"`
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devs := s.deviceMgr.List()
	views := make([]DeviceView, 0, len(devs))
	for _, d := range devs {
		views = append(views, DeviceView{
			DeviceID:    d.DeviceID,
			Name:        d.Name,
			IP:          d.IP,
			Protocol:    d.Protocol,
			Status:      d.Status,
			LastProbeAt: d.LastProbeAt,
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": views,
		"count":   len(views),
	})
}

type QueueOverview struct {
	DeviceID     string `json:"device_id"`
	PendingCount int    `json:"pending_count"`
}

func (s *Server) handleQueues(w http.ResponseWriter, r *http.Request) {
	devs := s.deviceMgr.List()
	views := make([]QueueOverview, 0, len(devs))
	total := 0
	for _, d := range devs {
		c := s.queue.PendingCount(d.DeviceID)
		views = append(views, QueueOverview{DeviceID: d.DeviceID, PendingCount: c})
		total += c
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"queues":       views,
		"total_pending": total,
	})
}

func (s *Server) handleQueueDetail(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "device_id")
	if deviceID == "" {
		s.writeError(w, http.StatusBadRequest, "DEVICE_NOT_FOUND", "device_id is required")
		return
	}
	if _, ok := s.deviceMgr.Get(deviceID); !ok {
		s.writeError(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "device not found")
		return
	}
	pending := s.queue.ListPending(deviceID)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":     deviceID,
		"pending_count": len(pending),
		"tasks":         pending,
	})
}

type LogEntry struct {
	TS    time.Time `json:"ts"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	level := r.URL.Query().Get("level")

	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	entries := readRecentLogs(s.cfg.Storage.LogDir, limit, level)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  entries,
		"count": len(entries),
	})
}

type ConfigView struct {
	ConfigVersion int                    `json:"config_version"`
	Cloud         CloudConfigView        `json:"cloud"`
	Ops           domain.OpsConfig       `json:"ops"`
	Storage       domain.StorageConfig   `json:"storage"`
	Log           domain.LogConfig       `json:"log"`
	Devices       []domain.Device        `json:"devices,omitempty"`
}

type CloudConfigView struct {
	Endpoint   string                 `json:"endpoint"`
	Protocol   domain.CloudProtocol   `json:"protocol"`
	DerivedURL string                 `json:"derived_url"`
	AgentID    string                 `json:"agent_id,omitempty"`
	DeviceToken string                `json:"device_token"`
	MTLSCert    string                `json:"mtls_cert,omitempty"`
	MTLSKey     string                `json:"mtls_key,omitempty"`
}

const maskedValue = "***"

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	view := ConfigView{
		ConfigVersion: s.cfg.ConfigVersion,
		Cloud: CloudConfigView{
			Endpoint:   s.cfg.Cloud.Endpoint,
			Protocol:   s.cfg.Cloud.Protocol,
			DerivedURL: s.cfg.Cloud.DerivedURL,
			AgentID:    s.cfg.Cloud.AgentID,
			DeviceToken: maskedValue,
			MTLSCert:    maskedValue,
			MTLSKey:     maskedValue,
		},
		Ops:     s.cfg.Ops,
		Storage: s.cfg.Storage,
		Log:     s.cfg.Log,
		Devices: s.cfg.Devices,
	}
	s.writeJSON(w, http.StatusOK, view)
}

var processStart = time.Now()