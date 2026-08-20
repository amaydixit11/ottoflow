/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	cliexec "github.com/nirmata/ottoflow/cli/internal/executor"
)

// withAllowInsecureURL sets the package-level --allow-insecure-url flag var for the duration
// of a test and restores it afterward, since fetchURL reads it as global CLI state.
func withAllowInsecureURL(t *testing.T, value bool) {
	t.Helper()
	old := allowInsecureURL
	allowInsecureURL = value
	t.Cleanup(func() { allowInsecureURL = old })
}

func TestReadRunSource_Stdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("stdin-content"))

	data, err := readRunSource(cmd, context.Background(), "-")
	if err != nil {
		t.Fatalf("readRunSource: %v", err)
	}
	if string(data) != "stdin-content" {
		t.Errorf("expected stdin-content, got %q", data)
	}
}

func TestReadRunSource_File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte("file-content"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	data, err := readRunSource(&cobra.Command{}, context.Background(), path)
	if err != nil {
		t.Fatalf("readRunSource: %v", err)
	}
	if string(data) != "file-content" {
		t.Errorf("expected file-content, got %q", data)
	}
}

func TestReadRunSource_FileNotFound(t *testing.T) {
	_, err := readRunSource(&cobra.Command{}, context.Background(), filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

// A mistyped Workflow body must surface as a clear parse error, whether the bytes came from a
// file, stdin, or a URL -- readRunSource just fetches bytes, LoadFromReader does the parsing.
func TestReadRunSource_BadYAMLSurfacedByLoadFromReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.yaml")
	broken := `apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: broken
spec:
  steps: "this-should-be-a-list"
`
	if err := os.WriteFile(path, []byte(broken), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	data, err := readRunSource(&cobra.Command{}, context.Background(), path)
	if err != nil {
		t.Fatalf("readRunSource: %v", err)
	}

	exec := cliexec.NewLocalWorkflowExecutor(nil, "", 5, "", "")
	err = exec.LoadFromReader(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected LoadFromReader to fail on a Workflow with a mistyped body")
	}
	if !strings.Contains(err.Error(), "parse Workflow") {
		t.Errorf("error should name the failed Workflow parse, got: %v", err)
	}
}

func TestFetchURL_RejectsInsecureHTTPWithoutFlag(t *testing.T) {
	withAllowInsecureURL(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached when the scheme is rejected up front")
	}))
	defer srv.Close()

	_, err := fetchURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected an error for a plain http:// URL without --allow-insecure-url")
	}
	if !strings.Contains(err.Error(), "allow-insecure-url") {
		t.Errorf("expected error to mention --allow-insecure-url, got: %v", err)
	}
}

func TestFetchURL_AllowsInsecureHTTPWithFlag(t *testing.T) {
	withAllowInsecureURL(t, true)
	const body = "apiVersion: ottoflow.nirmata.io/v1alpha1\nkind: Workflow\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	data, err := fetchURL(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchURL: %v", err)
	}
	if string(data) != body {
		t.Errorf("expected fetched body %q, got %q", body, data)
	}
}

func TestFetchURL_NonSuccessStatusRejected(t *testing.T) {
	withAllowInsecureURL(t, true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to mention the status, got: %v", err)
	}
}

func TestFetchURL_OversizeBodyRejected(t *testing.T) {
	withAllowInsecureURL(t, true)
	const maxURLBytes = 10 << 20 // must match fetchURL's own limit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, maxURLBytes+1))
	}))
	defer srv.Close()

	_, err := fetchURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected an error for a response over the 10 MiB limit")
	}
	if !strings.Contains(err.Error(), "10 MiB") {
		t.Errorf("expected error to mention the 10 MiB limit, got: %v", err)
	}
}

// fetchURL derives its own deadline from the caller's context, so a context that is already
// close to its deadline must cut a slow response short rather than hanging for 30s.
func TestFetchURL_ContextTimeout(t *testing.T) {
	withAllowInsecureURL(t, true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exit as soon as the client gives up, so the test doesn't wait out a fixed sleep.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := fetchURL(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected a timeout error for a response slower than the context deadline")
	}
}

// checkRedirectPolicy is exercised directly (no server needed) so the https->http downgrade
// rejection can be tested without standing up a real TLS listener -- fetchURL's http.Client
// uses the default Transport, which would refuse a self-signed httptest.NewTLSServer cert
// before ever reaching the redirect logic.
func TestCheckRedirectPolicy_RejectsHTTPRedirectWithoutFlag(t *testing.T) {
	withAllowInsecureURL(t, false)
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/next", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err := checkRedirectPolicy(req, nil); err == nil {
		t.Fatal("expected a redirect to a plain http:// URL to be rejected")
	} else if !strings.Contains(err.Error(), "allow-insecure-url") {
		t.Errorf("expected error to mention --allow-insecure-url, got: %v", err)
	}
}

func TestCheckRedirectPolicy_AllowsHTTPSRedirect(t *testing.T) {
	withAllowInsecureURL(t, false)
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/next", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err := checkRedirectPolicy(req, nil); err != nil {
		t.Errorf("expected an https redirect to be allowed, got: %v", err)
	}
}

func TestCheckRedirectPolicy_EnforcesHopCap(t *testing.T) {
	withAllowInsecureURL(t, true) // isolate the hop-cap check from the scheme check
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/next", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	via := make([]*http.Request, 10) // 10 redirects already followed
	if err := checkRedirectPolicy(req, via); err == nil {
		t.Fatal("expected the 11th request (10 redirects already followed) to be rejected")
	} else if !strings.Contains(err.Error(), "10 redirects") {
		t.Errorf("expected error to mention the redirect cap, got: %v", err)
	}
}

func TestCheckRedirectPolicy_AllowsUpToHopCap(t *testing.T) {
	withAllowInsecureURL(t, true)
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/next", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	via := make([]*http.Request, 9) // 9 redirects already followed; the 10th must still be allowed
	if err := checkRedirectPolicy(req, via); err != nil {
		t.Errorf("expected the 10th redirect to be allowed, got: %v", err)
	}
}

// End-to-end: a real redirect chain through fetchURL must stop once it exceeds the hop cap,
// instead of following it indefinitely or until some other limit kicks in.
func TestFetchURL_EnforcesRedirectHopCapEndToEnd(t *testing.T) {
	withAllowInsecureURL(t, true) // isolate the hop cap from the http/https scheme check
	var mux http.ServeMux
	const hops = 11 // one more than the cap: the client must give up before reaching /final
	for i := 0; i < hops; i++ {
		next := fmt.Sprintf("/%d", i+1)
		mux.HandleFunc(fmt.Sprintf("/%d", i), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, next, http.StatusFound)
		})
	}
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		t.Error("should never reach /final: the hop cap must stop the chain first")
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	_, err := fetchURL(context.Background(), srv.URL+"/0")
	if err == nil {
		t.Fatal("expected an error once the redirect chain exceeds the hop cap")
	}
	if !strings.Contains(err.Error(), "10 redirects") {
		t.Errorf("expected error to mention the redirect cap, got: %v", err)
	}
}

// resolveOptionalKubeClient must tolerate a genuinely absent kubeconfig (the -f/stdin path's
// zero-setup promise) but surface a real error for one that exists yet fails to load, instead of
// collapsing both into the same "no kubeconfig" outcome.
func withHomeAndKubeconfigFlag(t *testing.T, home, kubeconfigFlag string) {
	t.Helper()
	t.Setenv("HOME", home)
	oldFlag := kubeconfig
	kubeconfig = kubeconfigFlag
	t.Cleanup(func() { kubeconfig = oldFlag })
}

func TestResolveOptionalKubeClient_AbsentKubeconfigTolerated(t *testing.T) {
	home := t.TempDir() // no .kube/config under here
	withHomeAndKubeconfigFlag(t, home, "")

	config, k8sClient, err := resolveOptionalKubeClient()
	if err != nil {
		t.Fatalf("expected a genuinely absent kubeconfig to be tolerated, got: %v", err)
	}
	if config != nil || k8sClient != nil {
		t.Errorf("expected nil config and client, got config=%v client=%v", config, k8sClient)
	}
}

func TestResolveOptionalKubeClient_MalformedKubeconfigSurfacesError(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".kube"), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	badConfig := []byte("not: valid: kubeconfig: [yaml")
	if err := os.WriteFile(filepath.Join(home, ".kube", "config"), badConfig, 0600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	withHomeAndKubeconfigFlag(t, home, "")

	_, _, err := resolveOptionalKubeClient()
	if err == nil {
		t.Fatal("expected a malformed kubeconfig at the default path to surface an error")
	}
}
