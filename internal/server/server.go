// Package server serves the web client and negotiates WebRTC sessions.
//
// Signaling is one HTTP round trip (non-trickle ICE, similar to WHEP):
// the browser POSTs its offer to /api/session and receives the answer.
// Plain HTTP is enough because RTCPeerConnection does not require a secure
// context; media is encrypted by DTLS-SRTP.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/pairing"
	"github.com/naoking158/spareglass/internal/vdisplay"
	"github.com/naoking158/spareglass/web"
)

// Options configures a Server.
type Options struct {
	Config   config.Config
	Token    pairing.Token
	Provider vdisplay.Provider
	Logger   *slog.Logger
}

// Server is the HTTP front of the host.
type Server struct {
	opts     Options
	sessions *Manager
	log      *slog.Logger
}

// New builds a Server.
func New(o Options) *Server {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Server{opts: o, sessions: NewManager(o.Config, o.Provider, o.Logger), log: o.Logger}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(web.Files, ".")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/session", s.handleSession)
	return securityHeaders(mux)
}

// Close tears down every session and virtual display.
func (s *Server) Close() {
	s.sessions.CloseAll()
	if err := s.opts.Provider.CloseAll(); err != nil {
		s.log.Warn("closing virtual displays", "err", err)
	}
}

type sessionRequest struct {
	SDP string `json:"sdp"`
}

type sessionResponse struct {
	ID  string `json:"id"`
	SDP string `json:"sdp"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if !s.opts.Token.Check(pairing.FromRequest(r)) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing pairing token"})
		return
	}
	var req sessionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.SDP == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be {\"sdp\": \"<offer>\"}"})
		return
	}
	id, answer, err := s.sessions.Create(r.Context(), req.SDP)
	switch {
	case errors.Is(err, ErrTooManyClients):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	case err != nil:
		s.log.Error("create session", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not start session"})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{ID: id, SDP: answer})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
