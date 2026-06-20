package admin

import (
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/paqetpremium/paqetpremium/internal/config"
)

// redactedMark replaces secret values in config returned to clients. When a
// value comes back unchanged on save, the running secret is preserved.
const redactedMark = "***REDACTED***"

func (s *Server) configEditor() bool {
	return strings.TrimSpace(s.cfgPath) != "" && s.getConfig != nil
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getConfigYAML(w)
	case http.MethodPost, http.MethodPut:
		s.putConfigYAML(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getConfigYAML(w http.ResponseWriter) {
	cur := s.getConfig()
	if cur == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	red, err := redactConfig(cur)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out, err := yaml.Marshal(red)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	_, _ = w.Write(out)
}

func (s *Server) putConfigYAML(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var incoming config.Config
	if err := yaml.Unmarshal(body, &incoming); err != nil {
		http.Error(w, "parse yaml: "+err.Error(), http.StatusBadRequest)
		return
	}
	restoreSecrets(&incoming, s.getConfig())
	if err := incoming.Validate(); err != nil {
		http.Error(w, "invalid config: "+err.Error(), http.StatusBadRequest)
		return
	}
	out, err := yaml.Marshal(&incoming)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.writeConfigFile(out); err != nil {
		http.Error(w, "write config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if s.reload != nil {
		if err := s.reload(); err != nil {
			http.Error(w, "saved, but reload failed (restart may be required): "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]string{"status": "saved"})
}

func (s *Server) writeConfigFile(data []byte) error {
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(s.cfgPath); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := s.cfgPath + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, s.cfgPath)
}

func redactConfig(c *config.Config) (*config.Config, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	var cp config.Config
	if err := yaml.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cp.Transport.KCP.Key) != "" {
		cp.Transport.KCP.Key = redactedMark
	}
	if cp.Admin != nil && strings.TrimSpace(cp.Admin.Token) != "" {
		cp.Admin.Token = redactedMark
	}
	if cp.Upstream != nil {
		for i := range cp.Upstream.Servers {
			if strings.TrimSpace(cp.Upstream.Servers[i].Key) != "" {
				cp.Upstream.Servers[i].Key = redactedMark
			}
		}
	}
	for i := range cp.SOCKS5 {
		if cp.SOCKS5[i].Auth != nil && cp.SOCKS5[i].Auth.Pass != "" {
			cp.SOCKS5[i].Auth.Pass = redactedMark
		}
	}
	return &cp, nil
}

func restoreSecrets(in, cur *config.Config) {
	if cur == nil {
		return
	}
	if isRedacted(in.Transport.KCP.Key) {
		in.Transport.KCP.Key = cur.Transport.KCP.Key
	}
	if in.Admin != nil && cur.Admin != nil && isRedacted(in.Admin.Token) {
		in.Admin.Token = cur.Admin.Token
	}
	if in.Upstream != nil && cur.Upstream != nil {
		for i := range in.Upstream.Servers {
			if isRedacted(in.Upstream.Servers[i].Key) {
				if key := lookupUpstreamKey(cur.Upstream.Servers, in.Upstream.Servers[i].Name, i); key != "" {
					in.Upstream.Servers[i].Key = key
				}
			}
		}
	}
	for i := range in.SOCKS5 {
		if in.SOCKS5[i].Auth != nil && isRedacted(in.SOCKS5[i].Auth.Pass) {
			if i < len(cur.SOCKS5) && cur.SOCKS5[i].Auth != nil {
				in.SOCKS5[i].Auth.Pass = cur.SOCKS5[i].Auth.Pass
			}
		}
	}
}

func lookupUpstreamKey(servers []config.UpstreamServer, name string, idx int) string {
	if name != "" {
		for _, s := range servers {
			if s.Name == name {
				return s.Key
			}
		}
	}
	if idx >= 0 && idx < len(servers) {
		return servers[idx].Key
	}
	return ""
}

func isRedacted(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == redactedMark
}
