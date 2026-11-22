// Package server serves the web client, pairs browsers and negotiates WebRTC
// sessions.
//
// URLs are meant to be typed by hand: http://<host-ip>/1 is display 1,
// /2 is display 2, and so on. A browser seen for the first time asks to pair
// and the host owner approves it in the terminal; the browser then keeps a
// device credential and connects directly on later visits.
//
// Signaling is one HTTP round trip (non-trickle ICE, similar to WHEP).
// Plain HTTP is enough because RTCPeerConnection does not require a secure
// context; media is encrypted by DTLS-SRTP.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/media"
	"github.com/naoking158/spareglass/internal/pairing"
	"github.com/naoking158/spareglass/internal/vdisplay"
	"github.com/naoking158/spareglass/web"
)

// Options configures a Server.
type Options struct {
	Config   config.Config
	Pairer   *pairing.Pairer
	Provider vdisplay.Provider
	// NewPipeline builds capture pipelines; nil means media.NewPipeline.
	NewPipeline media.Factory
	Logger      *slog.Logger
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
	return &Server{opts: o, sessions: NewManager(o.Config, o.Provider, o.NewPipeline, o.Logger), log: o.Logger}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(web.Files, ".")
	files := http.FileServerFS(static)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/1", http.StatusFound)
	})
	mux.HandleFunc("GET /{slot}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.parseSlot(r.PathValue("slot")); !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, static, "index.html")
	})
	mux.Handle("GET /app.js", files)
	mux.Handle("GET /style.css", files)
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/pair", s.handlePairStart)
	mux.HandleFunc("GET /api/pair/{id}", s.handlePairStatus)
	mux.HandleFunc("POST /api/session", s.handleSession)
	return s.logRequests(securityHeaders(mux))
}

// logRequests logs every request at debug level (serve -v), so "the page did
// not load" can be told apart from "the request never reached the host".
func (s *Server) logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "from", remoteIP(r), "ua", r.UserAgent())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Close tears down every session and virtual display.
func (s *Server) Close() {
	s.sessions.CloseAll()
	if err := s.opts.Provider.CloseAll(); err != nil {
		s.log.Warn("closing virtual displays", "err", err)
	}
}

func (s *Server) parseSlot(v string) (int, bool) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > s.opts.Config.MaxClients {
		return 0, false
	}
	return n, true
}

type pairRequest struct {
	Name string `json:"name"`
	Slot int    `json:"slot"`
}

type pairStartResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

type pairStatusResponse struct {
	Status   pairing.State `json:"status"`
	DeviceID string        `json:"device_id,omitempty"`
	Secret   string        `json:"secret,omitempty"`
}

func (s *Server) handlePairStart(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body must be {\"name\": ..., \"slot\": ...}")
		return
	}
	if _, ok := s.parseSlot(strconv.Itoa(req.Slot)); !ok {
		writeError(w, http.StatusBadRequest, "no such display")
		return
	}
	name := sanitizeName(req.Name)
	pr, err := s.opts.Pairer.Start(name, remoteIP(r), req.Slot)
	if errors.Is(err, pairing.ErrTooManyRequests) {
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start pairing")
		return
	}
	s.log.Info("pairing requested", "name", name, "addr", pr.Addr, "code", pr.Code)
	writeJSON(w, http.StatusOK, pairStartResponse{ID: pr.ID, Code: pr.Code})
}

func (s *Server) handlePairStatus(w http.ResponseWriter, r *http.Request) {
	st, dev, secret, ok := s.opts.Pairer.Status(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown pairing request")
		return
	}
	writeJSON(w, http.StatusOK, pairStatusResponse{Status: st, DeviceID: dev, Secret: secret})
}

type sessionRequest struct {
	SDP  string `json:"sdp"`
	Slot int    `json:"slot"`
}

type sessionResponse struct {
	ID  string `json:"id"`
	SDP string `json:"sdp"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id, secret, ok := pairing.ParseAuthorization(r.Header.Get("Authorization"))
	var dev pairing.Device
	if ok {
		var err error
		dev, ok, err = s.opts.Pairer.Store().Verify(id, secret)
		if err != nil {
			s.log.Error("read paired devices", "err", err)
			writeError(w, http.StatusServiceUnavailable, "the host cannot read its list of paired browsers")
			return
		}
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "this browser is not paired")
		return
	}
	var req sessionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.SDP == "" {
		writeError(w, http.StatusBadRequest, "body must be {\"sdp\": \"<offer>\", \"slot\": n}")
		return
	}
	if _, ok := s.parseSlot(strconv.Itoa(req.Slot)); !ok {
		writeError(w, http.StatusBadRequest, "no such display")
		return
	}
	sid, answer, err := s.sessions.Create(r.Context(), req.SDP, req.Slot-1, dev)
	switch {
	case errors.Is(err, ErrSlotInUse):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		s.log.Error("create session", "err", err)
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	// The browser may have been forgotten while the session was negotiated;
	// CloseDevice only reaches sessions that were already registered.
	if _, ok, err := s.opts.Pairer.Store().Verify(id, secret); err != nil || !ok {
		s.sessions.Close(sid)
		if err != nil {
			s.log.Error("read paired devices", "err", err)
			writeError(w, http.StatusServiceUnavailable, "the host cannot read its list of paired browsers")
		} else {
			writeError(w, http.StatusUnauthorized, "this browser is not paired")
		}
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{ID: sid, SDP: answer})
}

func sanitizeName(n string) string {
	n = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(n))
	if len([]rune(n)) > 40 {
		n = string([]rune(n)[:40])
	}
	if n == "" {
		n = "Unknown browser"
	}
	return n
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
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
