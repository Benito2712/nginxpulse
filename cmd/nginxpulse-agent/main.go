package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type agentConfig struct {
	Server        string   `json:"server"`
	AccessKey     string   `json:"accessKey"`
	WebsiteID     string   `json:"websiteID"`
	SourceID      string   `json:"sourceID"`
	Paths         []string `json:"paths"`
	PollInterval  string   `json:"pollInterval"`
	BatchSize     int      `json:"batchSize"`
	FlushInterval string   `json:"flushInterval"`
}

type ingestRequest struct {
	WebsiteID string   `json:"website_id"`
	SourceID  string   `json:"source_id"`
	Lines     []string `json:"lines"`
}

type fileState struct {
	offset   int64
	lastSize int64
	partial  string
}

func main() {
	configPath := flag.String("config", "configs/nginxpulse_agent.json", "agent config path")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		logrus.WithError(err).Error("加载 agent 配置失败")
		os.Exit(1)
	}

	pollInterval := parseDuration(cfg.PollInterval, time.Second)
	flushInterval := parseDuration(cfg.FlushInterval, 2*time.Second)
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 200
	}
	sourceID := strings.TrimSpace(cfg.SourceID)
	if sourceID == "" {
		sourceID = "agent"
	}

	endpoint := strings.TrimRight(cfg.Server, "/") + "/api/ingest/logs"
	states := make(map[string]*fileState)
	pending := make([]string, 0, batchSize)

	pollTicker := time.NewTicker(pollInterval)
	flushTicker := time.NewTicker(flushInterval)
	defer pollTicker.Stop()
	defer flushTicker.Stop()

	for {
		select {
		case <-pollTicker.C:
			for _, path := range cfg.Paths {
				if strings.HasSuffix(strings.ToLower(path), ".gz") {
					continue
				}
				state := states[path]
				if state == nil {
					state = &fileState{}
					states[path] = state
				}
				lines, err := readNewLines(path, state)
				if err != nil {
					logrus.WithError(err).Warnf("读取日志失败: %s", path)
					continue
				}
				if len(lines) == 0 {
					continue
				}
				pending = append(pending, lines...)
				if len(pending) >= batchSize {
					if err := pushLines(endpoint, cfg.AccessKey, cfg.WebsiteID, sourceID, pending); err != nil {
						logrus.WithError(err).Warn("日志推送失败，将在下次重试")
						continue
					}
					pending = pending[:0]
				}
			}
		case <-flushTicker.C:
			if len(pending) == 0 {
				continue
			}
			if err := pushLines(endpoint, cfg.AccessKey, cfg.WebsiteID, sourceID, pending); err != nil {
				logrus.WithError(err).Warn("日志推送失败，将在下次重试")
				continue
			}
			pending = pending[:0]
		}
	}
}

func loadConfig(path string) (*agentConfig, error) {
	absPath := path
	if !filepath.IsAbs(path) {
		if cwd, err := os.Getwd(); err == nil {
			absPath = filepath.Join(cwd, path)
		}
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	cfg := &agentConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Server) == "" {
		return nil, errors.New("server 不能为空")
	}
	if strings.TrimSpace(cfg.WebsiteID) == "" {
		return nil, errors.New("websiteID 不能为空")
	}
	if len(cfg.Paths) == 0 {
		return nil, errors.New("paths 不能为空")
	}
	return cfg, nil
}

func readNewLines(path string, state *fileState) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size < state.offset {
		state.offset = 0
		state.partial = ""
	}
	if size == state.offset {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if _, err := file.Seek(state.offset, io.SeekStart); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(file)
	lines := []string{}
	for {
		line, err := reader.ReadString('\n')
		if len(line) == 0 && err != nil {
			break
		}
		state.offset += int64(len(line))
		line = strings.TrimRight(line, "\r\n")
		if state.partial != "" {
			line = state.partial + line
			state.partial = ""
		}
		if err == io.EOF {
			if line != "" {
				state.partial = line
			}
			break
		}
		if line != "" {
			lines = append(lines, line)
		}
		if err != nil {
			break
		}
	}
	state.lastSize = size
	return lines, nil
}

func pushLines(endpoint, accessKey, websiteID, sourceID string, lines []string) error {
	payload := ingestRequest{
		WebsiteID: websiteID,
		SourceID:  sourceID,
		Lines:     lines,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(accessKey) != "" {
		req.Header.Set("X-NginxPulse-Key", strings.TrimSpace(accessKey))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return nil
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return parsed
}
