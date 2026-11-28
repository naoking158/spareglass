package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/naoking158/spareglass/internal/netcheck"
	"github.com/naoking158/spareglass/internal/pairing"
	"github.com/naoking158/spareglass/web"
)

// TokenHeader carries the host page's token on every host API call.
const TokenHeader = "X-Spareglass-Token"

// HostOptions configures the host page.
type HostOptions struct {
	// Token must be sent in TokenHeader on every /api/ call.
	Token string
	// URLs returns the addresses display devices open now, e.g.
	// http://192.168.1.20/1. It is called on every state request, so the
	// page follows network changes. Required.
	URLs func() []string
	// DisplayProblem says why virtual displays cannot be created, if they cannot.
	DisplayProblem string
	// NetworkProblems returns settings found to stop display devices from
	// connecting. It is called on every state request, so it must be cheap.
	// Optional.
	NetworkProblems func() []netcheck.Problem
	// Stop shuts the host down, like Ctrl+C. Required.
	Stop func()
}

type hostState struct {
	URLs           []string           `json:"urls"`
	DisplayProblem string             `json:"display_problem,omitempty"`
	Network        []netcheck.Problem `json:"network_problems"`
	Pending        []hostPending      `json:"pending"`
	Devices        []hostDevice       `json:"devices"`
	Displays       []DisplayStatus    `json:"displays"`
}

type hostPending struct {
	ID      string    `json:"id"`
	Code    string    `json:"code"`
	Name    string    `json:"name"`
	Addr    string    `json:"addr"`
	Slot    int       `json:"slot"`
	Expires time.Time `json:"expires"`
}

type hostDevice struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
}

type answerRequest struct {
	Allow *bool `json:"allow"`
}

// HostHandler serves the page (and its API) the person at the host uses to
// see the connect URLs, approve browsers, forget them and stop the host.
// It approves pairing, so serve it only on a loopback listener: it also
// rejects requests that do not come from loopback, name a non-loopback Host
// (DNS rebinding) or carry another Origin (cross-site requests), and /api/
// calls without the token.
func (s *Server) HostHandler(o HostOptions) http.Handler {
	if o.Stop == nil || o.URLs == nil {
		panic("server: HostOptions.Stop and URLs are required")
	}
	static, _ := fs.Sub(web.Files, ".")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, static, "host.html")
	})
	mux.Handle("GET /host.js", http.FileServerFS(static))
	mux.Handle("GET /host.css", http.FileServerFS(static))
	mux.HandleFunc("GET /qr.png", func(w http.ResponseWriter, r *http.Request) {
		urls := o.URLs()
		if len(urls) == 0 {
			http.NotFound(w, r)
			return
		}
		png, err := qrcode.Encode(urls[0], qrcode.Medium, 256)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.hostState(o))
	})
	mux.HandleFunc("POST /api/pair/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req answerRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Allow == nil {
			writeError(w, http.StatusBadRequest, `body must be {"allow": true|false}`)
			return
		}
		err := s.opts.Pairer.Answer(r.PathValue("id"), *req.Allow)
		switch {
		case errors.Is(err, pairing.ErrNoSuchRequest):
			writeError(w, http.StatusNotFound, err.Error())
			return
		case err != nil:
			s.log.Error("answer pairing", "err", err)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.log.Info("pairing answered on the host page", "id", r.PathValue("id"), "allow", *req.Allow)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		err := s.opts.Pairer.Store().Remove(id)
		switch {
		case errors.Is(err, pairing.ErrUnknownDevice):
			writeError(w, http.StatusNotFound, err.Error())
			return
		case err != nil:
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.sessions.CloseDevice(id)
		s.log.Info("device forgotten on the host page", "device", id)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, _ *http.Request) {
		s.log.Info("stop requested on the host page")
		w.WriteHeader(http.StatusNoContent)
		go o.Stop()
	})
	return s.logRequests(securityHeaders(hostGuard(o.Token, mux)))
}

func (s *Server) hostState(o HostOptions) hostState {
	st := hostState{URLs: o.URLs(), DisplayProblem: o.DisplayProblem, Network: []netcheck.Problem{}, Pending: []hostPending{}, Devices: []hostDevice{}, Displays: s.sessions.Displays()}
	if o.NetworkProblems != nil {
		st.Network = append(st.Network, o.NetworkProblems()...)
	}
	for _, r := range s.opts.Pairer.Pending() {
		st.Pending = append(st.Pending, hostPending{ID: r.ID, Code: r.Code, Name: r.Name, Addr: r.Addr, Slot: r.Slot, Expires: r.Created.Add(pairing.RequestTTL)})
	}
	for _, d := range s.opts.Pairer.Store().List() {
		st.Devices = append(st.Devices, hostDevice{ID: d.ID, Name: d.Name, Created: d.Created})
	}
	return st
}

func hostGuard(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := net.ParseIP(remoteIP(r)); ip == nil || !ip.IsLoopback() {
			writeError(w, http.StatusForbidden, "the host page is only available on this computer")
			return
		}
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" {
			writeError(w, http.StatusForbidden, "unexpected Host")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			writeError(w, http.StatusForbidden, "unexpected Origin")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && (token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(TokenHeader)), []byte(token)) != 1) {
			writeError(w, http.StatusForbidden, "missing or wrong token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
