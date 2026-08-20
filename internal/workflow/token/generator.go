/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package token

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

const (
	// TokenPrefix is the prefix for callback tokens
	TokenPrefix = "cb_"

	// TokenLength is the length of the random part (in bytes).
	// 32 bytes = 256 bits of entropy; encoded as 64 lowercase hex chars.
	TokenLength = 32

	// Base62Chars are the characters used for base62 encoding
	Base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// Generator generates cryptographically secure callback tokens
type Generator struct {
	rand io.Reader
}

// NewGenerator creates a new token generator using crypto/rand
func NewGenerator() *Generator {
	return &Generator{
		rand: rand.Reader,
	}
}

// Generate creates a new callback token.
// Returns the plaintext token (cb_ + 64 lowercase hex chars = 32 bytes / 256 bits of entropy) and its SHA256 hash.
// Only the hash should be persisted; the plaintext token must never be stored in K8s status.
func (g *Generator) Generate() (token string, hash string, err error) {
	randomBytes := make([]byte, TokenLength)
	if _, err := io.ReadFull(g.rand, randomBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// All 32 bytes encoded as 64 lowercase hex chars = 256 bits of entropy
	encoded := fmt.Sprintf("%x", randomBytes)

	token = TokenPrefix + encoded

	// Create hash for secure storage (never store plain token)
	h := sha256.Sum256([]byte(token))
	hash = fmt.Sprintf("%x", h[:])

	return token, hash, nil
}

// HashToken returns the SHA256 hash of a token (for secure storage/comparison)
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h[:])
}

// ValidateToken checks if a token is in the expected format (cb_ + 64 lowercase hex chars).
// Only lowercase hex is accepted since Generate() uses fmt.Sprintf("%x").
func ValidateToken(token string) bool {
	if len(token) != len(TokenPrefix)+64 {
		return false
	}
	if token[:len(TokenPrefix)] != TokenPrefix {
		return false
	}
	for _, c := range token[len(TokenPrefix):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
