package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/paqetpremium/paqetpremium/internal/version"
)

const authHeader = "Authorization"

type Status struct {
	Core      string      `json:"core"`
	Version   string      `json:"version"`
	Role      string      `json:"role"`
	Name      string      `json:"name"`
	Strategy  string      `json:"strategy,omitempty"`
	Active    string      `json:"active_upstream,omitempty"`
	Listen    string      `json:"listen_port,omitempty"`
	Sessions  int                  `json:"sessions,omitempty"`
	Upstreams interface{}          `json:"upstreams,omitempty"`
	Stats     *metrics.Snapshot    `json:"stats,omitempty"`
}

type Server struct {
	cfg    *config.AdminConfig
	log    *slog.Logger
	status func() Status
	reload func() error
}

func New(cfg *config.AdminConfig, log *slog.Logger, status func() Status, reload func() error) *Server {
	return &Server{cfg: cfg, log: log, status: status, reload: reload}
}

func (s *Server) Run(ctx context.Context) error {
	if s.cfg == nil || strings.TrimSpace(s.cfg.Listen) == "" {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/v1/status", s.auth(s.handleStatus))
	if s.reload != nil {
		mux.HandleFunc("/api/v1/reload", s.auth(s.handleReload))
	}
	if s.cfg.Metrics {
		mux.HandleFunc("/metrics", s.auth(s.handleMetrics))
	}

	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	s.log.Info("admin API listening", "addr", s.cfg.Listen, "auth", s.cfg.Token != "")
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	if s.cfg.Token == "" {
		return h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	token := strings.TrimSpace(s.cfg.Token)
	if token == "" {
		return true
	}
	if q := strings.TrimSpace(r.URL.Query().Get("token")); q == token {
		return true
	}
	hdr := strings.TrimSpace(r.Header.Get(authHeader))
	if strings.HasPrefix(hdr, "Bearer ") {
		return strings.TrimPrefix(hdr, "Bearer ") == token
	}
	return hdr == token
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	st := s.status()
	if st.Core == "" {
		st.Core = version.Name
	}
	if st.Version == "" {
		st.Version = version.Version
	}
	writeJSON(w, st)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.reload == nil {
		http.Error(w, "reload not supported", http.StatusNotImplemented)
		return
	}
	if err := s.reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "reloaded"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	st := s.status()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	snap := metrics.Snapshot{}
	if st.Stats != nil {
		snap = *st.Stats
	} else {
		snap = metrics.Default.Snapshot()
	}
	metrics.WritePrometheus(w, snap, st.Sessions, st.Upstreams)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
