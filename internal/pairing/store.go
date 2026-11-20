// Package pairing decides which browsers may use this host as a display.
//
// A browser that is not yet known asks to pair; the host owner approves it
// (see Pairer and Approver). An approved browser receives a device id and a
// secret that it keeps in localStorage, so later visits connect without asking.
// The host stores only a SHA-256 of each secret, in devices.json.
package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Device is a browser that was approved once.
type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	SecretHash string    `json:"secret_hash"`
	Created    time.Time `json:"created"`
}

// Store keeps approved devices in a JSON file. It reloads the file when it
// changes on disk, so "spareglass devices revoke" applies to a running host.
type Store struct {
	path string

	mu      sync.Mutex
	devices map[string]Device
	modTime time.Time
}

// ErrUnknownDevice is returned when an id is not in the store.
var ErrUnknownDevice = errors.New("unknown device")

// OpenStore loads path; a missing file means no devices yet.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, devices: map[string]Device{}}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s, s.loadLocked()
}

func (s *Store) loadLocked() error {
	fi, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.devices = map[string]Device{}
		s.modTime = time.Time{}
		return nil
	}
	if err != nil {
		return err
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var list []Device
	if len(b) > 0 {
		if err := json.Unmarshal(b, &list); err != nil {
			return fmt.Errorf("parse %s: %w", s.path, err)
		}
	}
	s.devices = make(map[string]Device, len(list))
	for _, d := range list {
		s.devices[d.ID] = d
	}
	s.modTime = fi.ModTime()
	return nil
}

// refreshLocked reloads the file if it changed on disk. On error the
// in-memory list is stale and must not be trusted or written back.
func (s *Store) refreshLocked() error {
	fi, err := os.Stat(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.devices = map[string]Device{}
		s.modTime = time.Time{}
		return nil
	case err != nil:
		return err
	case fi.ModTime().Equal(s.modTime):
		return nil
	}
	return s.loadLocked()
}

func (s *Store) saveLocked() error {
	list := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.Before(list[j].Created) })
	b, _ := json.MarshalIndent(list, "", "  ")
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if fi, err := os.Stat(s.path); err == nil {
		s.modTime = fi.ModTime()
	}
	return nil
}

// Add registers a device and returns its id and secret. The secret is not
// kept; only its hash is stored.
func (s *Store) Add(name string) (id, secret string, err error) {
	secret = randomToken(24)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return "", "", err
	}
	for id = randomHex(4); ; id = randomHex(4) {
		if _, taken := s.devices[id]; !taken {
			break
		}
	}
	s.devices[id] = Device{ID: id, Name: name, SecretHash: hashSecret(secret), Created: time.Now().UTC()}
	return id, secret, s.saveLocked()
}

// Verify reports whether secret belongs to device id. A non-nil error means
// the store could not be read; the answer is then false but says nothing about
// whether the device is paired.
func (s *Store) Verify(id, secret string) (Device, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return Device{}, false, err
	}
	d, ok := s.devices[id]
	if !ok || secret == "" {
		return Device{}, false, nil
	}
	return d, subtle.ConstantTimeCompare([]byte(d.SecretHash), []byte(hashSecret(secret))) == 1, nil
}

// List returns devices, oldest first.
func (s *Store) List() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.refreshLocked()
	list := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.Before(list[j].Created) })
	return list
}

// Remove forgets one device.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	if _, ok := s.devices[id]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownDevice, id)
	}
	delete(s.devices, id)
	return s.saveLocked()
}

// Clear forgets every device.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = map[string]Device{}
	return s.saveLocked()
}

// ParseAuthorization reads "Authorization: Device <id>:<secret>".
func ParseAuthorization(h string) (id, secret string, ok bool) {
	v, found := strings.CutPrefix(h, "Device ")
	if !found {
		return "", "", false
	}
	id, secret, ok = strings.Cut(strings.TrimSpace(v), ":")
	return id, secret, ok && id != "" && secret != ""
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
