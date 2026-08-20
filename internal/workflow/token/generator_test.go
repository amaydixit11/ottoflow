/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package token_test

import (
	"strings"
	"testing"

	"github.com/nirmata/ottoflow/internal/workflow/token"
)

func TestGenerate_Format(t *testing.T) {
	g := token.NewGenerator()
	tok, hash, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.HasPrefix(tok, token.TokenPrefix) {
		t.Errorf("token %q does not start with prefix %q", tok, token.TokenPrefix)
	}
	expectedLen := len(token.TokenPrefix) + 64
	if len(tok) != expectedLen {
		t.Errorf("token length %d, want %d", len(tok), expectedLen)
	}
	if hash == "" {
		t.Error("hash is empty")
	}
	if len(hash) != 64 {
		t.Errorf("hash length %d, want 64 (SHA256 hex)", len(hash))
	}
}

func TestGenerate_Unique(t *testing.T) {
	g := token.NewGenerator()
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tok, _, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate() error on iteration %d: %v", i, err)
		}
		if seen[tok] {
			t.Errorf("duplicate token generated: %s", tok)
		}
		seen[tok] = true
	}
}

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"valid", "cb_" + strings.Repeat("a", 64), true},
		{"valid hex mixed", "cb_" + strings.Repeat("0123456789abcdef", 4), true},
		{"too short", "cb_abc", false},
		{"too long", "cb_" + strings.Repeat("a", 65), false},
		{"wrong prefix", "xx_" + strings.Repeat("a", 64), false},
		{"invalid chars", "cb_" + strings.Repeat("z", 64), false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := token.ValidateToken(tt.token)
			if got != tt.valid {
				t.Errorf("ValidateToken(%q) = %v, want %v", tt.token, got, tt.valid)
			}
		})
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	tok := "cb_" + strings.Repeat("a", 64)
	h1 := token.HashToken(tok)
	h2 := token.HashToken(tok)
	if h1 != h2 {
		t.Errorf("HashToken is not deterministic: %q vs %q", h1, h2)
	}
	if h1 == tok {
		t.Error("HashToken returned the input unchanged")
	}
}

func TestGenerate_TokenPassesValidation(t *testing.T) {
	g := token.NewGenerator()
	for i := 0; i < 20; i++ {
		tok, _, err := g.Generate()
		if err != nil {
			t.Fatalf("Generate() error: %v", err)
		}
		if !token.ValidateToken(tok) {
			t.Errorf("generated token %q fails ValidateToken", tok)
		}
	}
}
