/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package webhook

import (
	"context"
	"errors"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// fakeReviewer records the SubjectAccessReview it was asked for and answers
// with a fixed verdict.
type fakeReviewer struct {
	allowed bool
	reason  string
	err     error
	got     *authorizationv1.SubjectAccessReview
}

func (f *fakeReviewer) Create(_ context.Context, sar *authorizationv1.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
	f.got = sar
	if f.err != nil {
		return nil, f.err
	}
	out := sar.DeepCopy()
	out.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: f.allowed, Reason: f.reason}
	return out, nil
}

func runWithServiceAccount(name string) *ottoflowv1alpha1.WorkflowRun {
	run := &ottoflowv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "run-1"},
		Spec: ottoflowv1alpha1.WorkflowRunSpec{
			WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf"},
		},
	}
	if name != "" {
		run.Spec.Execution = &ottoflowv1alpha1.WorkflowRunExecutionSpec{
			Job: &ottoflowv1alpha1.WorkflowRunJobSpec{ServiceAccountName: name},
		}
	}
	return run
}

func requestContext(groups ...string) context.Context {
	return admission.NewContextWithRequest(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authenticationv1.UserInfo{
				Username: "alice",
				UID:      "uid-1",
				Groups:   groups,
				Extra:    map[string]authenticationv1.ExtraValue{"scopes": {"a", "b"}},
			},
		},
	})
}

// Naming a ServiceAccount is an escalation unless the submitter is allowed to
// run as it: the runner Job is launched with that account's token.
func TestRunnerServiceAccountRequiresAuthorization(t *testing.T) {
	reviewer := &fakeReviewer{allowed: false, reason: "no binding"}
	v := &WorkflowRunValidator{Authorizer: reviewer}

	_, err := v.ValidateCreate(requestContext(), runWithServiceAccount("privileged-sa"))
	if err == nil {
		t.Fatal("run naming an unauthorized ServiceAccount was admitted")
	}
	for _, want := range []string{"alice", "privileged-sa", "team-a", "no binding"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestRunnerServiceAccountAllowedWhenAuthorized(t *testing.T) {
	reviewer := &fakeReviewer{allowed: true}
	v := &WorkflowRunValidator{Authorizer: reviewer}

	if _, err := v.ValidateCreate(requestContext("builders"), runWithServiceAccount("build-runner")); err != nil {
		t.Fatalf("authorized run rejected: %v", err)
	}

	spec := reviewer.got.Spec
	if spec.User != "alice" || spec.UID != "uid-1" {
		t.Errorf("review asked about %q/%q, want alice/uid-1", spec.User, spec.UID)
	}
	if len(spec.Groups) != 1 || spec.Groups[0] != "builders" {
		t.Errorf("review groups = %v, want [builders]", spec.Groups)
	}
	if len(spec.Extra["scopes"]) != 2 {
		t.Errorf("review extra = %v, want the submitter's extra carried through", spec.Extra)
	}
	attrs := spec.ResourceAttributes
	if attrs == nil {
		t.Fatal("review carried no resource attributes")
	}
	if attrs.Verb != UseVerb || attrs.Resource != "serviceaccounts" ||
		attrs.Name != "build-runner" || attrs.Namespace != "team-a" {
		t.Errorf("review asked for %+v, want use serviceaccounts/build-runner in team-a", attrs)
	}
}

// A run that names no ServiceAccount gets the least-privilege default, so
// there is nothing to authorize and no review to spend.
func TestNoServiceAccountNoReview(t *testing.T) {
	reviewer := &fakeReviewer{allowed: false}
	v := &WorkflowRunValidator{Authorizer: reviewer}

	if _, err := v.ValidateCreate(requestContext(), runWithServiceAccount("")); err != nil {
		t.Fatalf("run without a ServiceAccount rejected: %v", err)
	}
	if reviewer.got != nil {
		t.Error("a review was issued for a run that names no ServiceAccount")
	}
}

// The field is mutable, so authorizing only creates would let a run be
// submitted clean and then patched.
func TestRunnerServiceAccountCheckedOnUpdate(t *testing.T) {
	v := &WorkflowRunValidator{Authorizer: &fakeReviewer{allowed: false}}

	_, err := v.ValidateUpdate(requestContext(), runWithServiceAccount(""), runWithServiceAccount("privileged-sa"))
	if err == nil {
		t.Fatal("a run patched to name an unauthorized ServiceAccount was admitted")
	}
}

// Anything that stops the check from reaching a verdict is a rejection, not a
// pass: admitting on a failed review would make the check decoration.
func TestRunnerServiceAccountFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		v    *WorkflowRunValidator
		ctx  context.Context
		want string
	}{
		{
			name: "review errors",
			v:    &WorkflowRunValidator{Authorizer: &fakeReviewer{err: errors.New("apiserver down")}},
			ctx:  requestContext(),
			want: "apiserver down",
		},
		{
			name: "no authorizer wired",
			v:    &WorkflowRunValidator{},
			ctx:  requestContext(),
			want: "cannot authorize",
		},
		{
			name: "request carries no user",
			v:    &WorkflowRunValidator{Authorizer: &fakeReviewer{allowed: true}},
			ctx:  context.Background(),
			want: "no user to authorize",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.v.ValidateCreate(tc.ctx, runWithServiceAccount("privileged-sa"))
			if err == nil {
				t.Fatal("run was admitted despite the check not reaching a verdict")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}
