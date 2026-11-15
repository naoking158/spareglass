// Package pairing guards access to the host with a shared token.
//
// The token travels in the URL fragment (#t=...), so it never appears in
// request lines or proxy logs; the web client sends it as a bearer token.
package pairing

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// Token is a shared secret between the host and its displays.
type Token string

// NewToken returns a random URL-safe token (144 bits).
func NewToken() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Check compares in constant time.
func (t Token) Check(got string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(t), []byte(got)) == 1
}

// FromRequest extracts "Authorization: Bearer <token>".
func FromRequest(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if v, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
