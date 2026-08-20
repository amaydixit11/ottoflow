/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
)

func TestCreateTLSServerUsesDynamicCert(t *testing.T) {
	called := false
	getCert := func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		called = true
		return nil, nil
	}

	srv := createTLSServer(":0", http.NewServeMux(), getCert)

	if srv.TLSConfig == nil {
		t.Fatal("TLSConfig is nil")
	}
	if srv.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want TLS 1.2 (%#x)", srv.TLSConfig.MinVersion, tls.VersionTLS12)
	}
	if srv.TLSConfig.GetCertificate == nil {
		t.Fatal("GetCertificate not wired; cert cannot hot-reload after rotation")
	}
	if _, err := srv.TLSConfig.GetCertificate(nil); err != nil {
		t.Fatalf("GetCertificate returned error: %v", err)
	}
	if !called {
		t.Error("GetCertificate did not delegate to the provided function")
	}
}

// writeCert writes a fresh self-signed cert/key pair with the given serial to
// certPath/keyPath, overwriting any existing files. Distinct serials let a test
// tell one generation of the cert from the next.
func writeCert(t *testing.T, certPath, keyPath string, serial int64) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "ottoflow-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

// TestCreateTLSServerServesRotatedCert proves the hot-reload contract end to end
// through ottoflow's own wiring: the certificate served by the http.Server that
// createTLSServer builds tracks the cert on disk after a rotation, without
// rebuilding the server. certwatcher.GetCertificate serves a cached cert, so a
// rotation is only reflected once the watcher re-reads disk — ReadCertificate()
// does that synchronously, keeping this test free of goroutines and timers.
func TestCreateTLSServerServesRotatedCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")

	writeCert(t, certPath, keyPath, 1001)

	cw, err := certwatcher.New(certPath, keyPath)
	if err != nil {
		t.Fatalf("certwatcher.New: %v", err)
	}

	// Read the served cert the same way the TLS stack does during a handshake:
	// through the server that createTLSServer wires up.
	srv := createTLSServer(":0", http.NewServeMux(), cw.GetCertificate)
	servedSerial := func() *big.Int {
		t.Helper()
		crt, err := srv.TLSConfig.GetCertificate(nil)
		if err != nil {
			t.Fatalf("GetCertificate: %v", err)
		}
		leaf, err := x509.ParseCertificate(crt.Certificate[0])
		if err != nil {
			t.Fatalf("parse served cert: %v", err)
		}
		return leaf.SerialNumber
	}

	if got := servedSerial(); got.Cmp(big.NewInt(1001)) != 0 {
		t.Fatalf("before rotation: served serial = %s, want 1001", got)
	}

	// Rotate the cert on disk to a new serial.
	writeCert(t, certPath, keyPath, 2002)

	// The watcher caches: until it re-reads disk, the old cert is still served.
	// Asserting this proves the reload below is what drives the change.
	if got := servedSerial(); got.Cmp(big.NewInt(1001)) != 0 {
		t.Fatalf("served serial changed to %s before reload; want still 1001", got)
	}

	if err := cw.ReadCertificate(); err != nil {
		t.Fatalf("ReadCertificate after rotation: %v", err)
	}

	if got := servedSerial(); got.Cmp(big.NewInt(2002)) != 0 {
		t.Fatalf("after rotation: served serial = %s, want 2002 (rotated cert not served)", got)
	}
}
