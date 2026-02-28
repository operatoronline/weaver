package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/operatoronline/weaver/pkg/agent"
	"github.com/operatoronline/weaver/pkg/logger"
	"github.com/operatoronline/weaver/pkg/tools"
)

type Server struct {
	server    *http.Server
	mu        sync.RWMutex
	ready     bool
	checks    map[string]Check
	startTime time.Time
	agent     *agent.AgentLoop
}

type Check struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type StatusResponse struct {
	Status string           `json:"status"`
	Uptime string           `json:"uptime"`
	Checks map[string]Check `json:"checks,omitempty"`
}

type ChatRequest struct {
	SessionKey string `json:"session_key"`
	Message    string `json:"message"`
	Channel    string `json:"channel"`
	ChatID     string `json:"chat_id"`
	// For image generation and other media tasks
	MediaConfig      map[string]interface{} `json:"media_config,omitempty"`
	Attachments      []map[string]string    `json:"attachments,omitempty"`
	ResponseMimeType string                 `json:"response_mime_type,omitempty"`
}

type ChatResponse struct {
	Response      string            `json:"response"`
	UICommands    []tools.UICommand `json:"ui_commands,omitempty"`
	AttachmentURL string            `json:"attachment_url,omitempty"`
	Error         string            `json:"error,omitempty"`
}

func NewServer(host string, port int, agentLoop *agent.AgentLoop) *Server {
	mux := http.NewServeMux()
	s := &Server{
		ready:     false,
		checks:    make(map[string]Check),
		startTime: time.Now(),
		agent:     agentLoop,
	}

	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/ready", s.readyHandler)
	mux.HandleFunc("/chat", s.chatHandler)
	mux.HandleFunc("/admin/status", s.statusHandler)
	mux.HandleFunc("/admin/service", s.serviceHandler)
	mux.HandleFunc("/admin/logs", s.logsHandler)

	addr := fmt.Sprintf("%s:%d", host, port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  300 * time.Second, // Increased for long generation turns
		WriteTimeout: 300 * time.Second,
	}

	return s
}

func (s *Server) Start() error {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	return s.server.ListenAndServe()
}

func (s *Server) StartContext(ctx context.Context) error {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.server.Shutdown(context.Background())
	}
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.ready = false
	s.mu.Unlock()
	return s.server.Shutdown(ctx)
}

func (s *Server) SetReady(ready bool) {
	s.mu.Lock()
	s.ready = ready
	s.mu.Unlock()
}

func (s *Server) RegisterCheck(name string, checkFn func() (bool, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, msg := checkFn()
	s.checks[name] = Check{
		Name:      name,
		Status:    statusString(status),
		Message:   msg,
		Timestamp: time.Now(),
	}
}

func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	// Simple authorization: check for API key in header (optional, but good for admin)
	// For now, allow all, but CORS will restrict to allowed domains
	status := s.agent.GetSystemStatus()
	status["uptime"] = time.Since(s.startTime).String()
	status["ready"] = s.ready

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) serviceHandler(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("systemctl", "status", "weaver").CombinedOutput()
	result := map[string]string{
		"output": string(out),
	}
	if err != nil {
		result["error"] = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) logsHandler(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("journalctl", "-u", "weaver", "-n", "100", "--no-pager").CombinedOutput()
	result := map[string]string{
		"output": string(out),
	}
	if err != nil {
		result["error"] = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Empty message", http.StatusBadRequest)
		return
	}

	if req.SessionKey == "" {
		req.SessionKey = "rest:default"
	}
	if req.Channel == "" {
		req.Channel = "rest"
	}
	if req.ChatID == "" {
		req.ChatID = "api"
	}

	logger.InfoCF("rest", "Received chat request", map[string]interface{}{
		"session_key":  req.SessionKey,
		"channel":      req.Channel,
		"chat_id":      req.ChatID,
		"message":      req.Message,
		"has_media":    req.MediaConfig != nil,
		"attachments":  len(req.Attachments),
	})

	response, uiCommands, err := s.agent.ProcessDirectWithChannel(r.Context(), req.Message, req.SessionKey, req.Channel, req.ChatID, req.MediaConfig, req.ResponseMimeType)

	resp := ChatResponse{
		Response:   response,
		UICommands: uiCommands,
	}
	if err != nil {
		resp.Error = err.Error()
	}

	// Extract attachment from response if it was an image generation
	if strings.Contains(response, "IMAGE_GENERATED:") {
		parts := strings.Split(response, "IMAGE_GENERATED:")
		if len(parts) > 1 {
			resp.AttachmentURL = strings.TrimSpace(parts[1])
			resp.Response = strings.TrimSpace(parts[0])
		}
	} else if strings.HasPrefix(response, "data:image") || strings.HasPrefix(response, "http") {
		// If it's just a URL/data URI, treat it as the attachment
		resp.AttachmentURL = response
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	uptime := time.Since(s.startTime)
	resp := StatusResponse{
		Status: "ok",
		Uptime: uptime.String(),
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.mu.RLock()
	ready := s.ready
	checks := make(map[string]Check)
	for k, v := range s.checks {
		checks[k] = v
	}
	s.mu.RUnlock()

	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(StatusResponse{
			Status: "not ready",
			Checks: checks,
		})
		return
	}

	for _, check := range checks {
		if check.Status == "fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(StatusResponse{
				Status: "not ready",
				Checks: checks,
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	uptime := time.Since(s.startTime)
	json.NewEncoder(w).Encode(StatusResponse{
		Status: "ready",
		Uptime: uptime.String(),
		Checks: checks,
	})
}

func statusString(ok bool) string {
	if ok {
		return "ok"
	}
	return "fail"
}
