// Package platform contains small cross-cutting helpers.
package platform

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// NewID returns a prefixed, URL-safe random identifier.
func NewID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return prefix + "_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "_" + hex.EncodeToString(b)
}

// NewToken returns a cryptographically secure random hex token.
func NewToken(bytes int) string {
	if bytes < 16 {
		bytes = 16
	}
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return NewID("tok") + NewID("")
	}
	return hex.EncodeToString(b)
}
