package opsapi

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultLogFilename = "agent.log"
	maxLogScanLines    = 10000
)

type rawLogEntry struct {
	TS    time.Time `json:"ts"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
	Raw   string    `json:"-"`
}

func readRecentLogs(logDir string, limit int, level string) []LogEntry {
	if logDir == "" {
		return []LogEntry{}
	}
	path := filepath.Join(logDir, defaultLogFilename)
	entries, err := scanLogFile(path, level)
	if err != nil {
		return []LogEntry{}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TS.After(entries[j].TS)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	result := make([]LogEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, LogEntry{TS: e.TS, Level: e.Level, Msg: e.Msg})
	}
	return result
}

func scanLogFile(path string, level string) ([]rawLogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []rawLogEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		count++
		if count > maxLogScanLines {
			break
		}
		line := scanner.Text()
		if line == "" {
			continue
		}
		entry, ok := parseLogLine(line)
		if !ok {
			continue
		}
		if level != "" && !strings.EqualFold(entry.Level, level) {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

func parseLogLine(line string) (rawLogEntry, bool) {
	if len(line) < 2 || line[0] != '{' {
		return rawLogEntry{Msg: line, Level: "info", TS: time.Now().UTC()}, true
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return rawLogEntry{Msg: line}, true
	}
	entry := rawLogEntry{Raw: line}
	if v, ok := m["level"].(string); ok {
		entry.Level = v
	}
	if v, ok := m["msg"].(string); ok {
		entry.Msg = v
	}
	if v, ok := m["ts"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			entry.TS = t
		} else if t, err := time.Parse(time.RFC3339, v); err == nil {
			entry.TS = t
		}
	}
	if entry.TS.IsZero() {
		entry.TS = time.Now().UTC()
	}
	if entry.Level == "" {
		entry.Level = "info"
	}
	return entry, true
}
