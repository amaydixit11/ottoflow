/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import "testing"

func TestIsClusterLocalHTTPHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		// cluster-local: plaintext http is acceptable
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"foo.svc", true},
		{"a.b.svc.cluster.local", true},
		{"FOO.SVC", true},                // case-insensitive
		{"foo.svc.cluster.local.", true}, // trailing FQDN dot
		// not cluster-local: must fail closed
		{"example.com", false},
		{"10.0.0.1", false},
		{"evilsvc.com", false},      // ".svc" is not a suffix here
		{"foo.svc.evil.com", false}, // ".svc" mid-host, not a suffix
	}
	for _, tc := range cases {
		if got := isClusterLocalHTTPHost(tc.host); got != tc.want {
			t.Errorf("isClusterLocalHTTPHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}
