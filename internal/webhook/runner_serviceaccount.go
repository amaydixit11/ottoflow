/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package webhook

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// UseVerb is the verb a subject needs on a ServiceAccount to run a workflow as
// it. Grant it with a Role naming the ServiceAccounts a subject may borrow:
//
//	rules:
//	- apiGroups: [""]
//	  resources: ["serviceaccounts"]
//	  resourceNames: ["build-runner"]
//	  verbs: ["use"]
const UseVerb = "use"

// SubjectAccessReviewer creates SubjectAccessReviews.
// kubernetes.Interface's AuthorizationV1().SubjectAccessReviews() satisfies it.
type SubjectAccessReviewer interface {
	Create(ctx context.Context, sar *authorizationv1.SubjectAccessReview, opts metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error)
}

// authorizeRunnerServiceAccount checks that whoever submitted this WorkflowRun
// may run workloads as the ServiceAccount it names.
//
// Without it, spec.execution.job.serviceAccountName is an escalation: the
// runner Job is launched with that ServiceAccount's token, so anyone who can
// create a WorkflowRun can name a privileged ServiceAccount and get its
// credentials, which is what scoping the runner to its own least-privilege
// role set out to prevent.
func (v *WorkflowRunValidator) authorizeRunnerServiceAccount(ctx context.Context, run *ottoflowv1alpha1.WorkflowRun) error {
	serviceAccount := runnerServiceAccountName(run)
	if serviceAccount == "" {
		return nil
	}

	if v.Authorizer == nil {
		return fmt.Errorf("WorkflowRun %q sets spec.execution.job.serviceAccountName but the validator cannot authorize it", run.Name)
	}

	request, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("WorkflowRun %q sets spec.execution.job.serviceAccountName but the request carries no user to authorize: %w", run.Name, err)
	}

	extra := map[string]authorizationv1.ExtraValue{}
	for key, values := range request.UserInfo.Extra {
		extra[key] = authorizationv1.ExtraValue(values)
	}

	review, err := v.Authorizer.Create(ctx, &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   request.UserInfo.Username,
			UID:    request.UserInfo.UID,
			Groups: request.UserInfo.Groups,
			Extra:  extra,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: run.Namespace,
				Verb:      UseVerb,
				Resource:  "serviceaccounts",
				Name:      serviceAccount,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("WorkflowRun %q: checking whether %q may use serviceaccount %q: %w",
			run.Name, request.UserInfo.Username, serviceAccount, err)
	}
	if !review.Status.Allowed {
		return fmt.Errorf("WorkflowRun %q: %q may not use serviceaccount %q in namespace %q%s",
			run.Name, request.UserInfo.Username, serviceAccount, run.Namespace, reviewReason(review))
	}
	return nil
}

func runnerServiceAccountName(run *ottoflowv1alpha1.WorkflowRun) string {
	if run.Spec.Execution == nil || run.Spec.Execution.Job == nil {
		return ""
	}
	return run.Spec.Execution.Job.ServiceAccountName
}

func reviewReason(review *authorizationv1.SubjectAccessReview) string {
	if review.Status.Reason == "" {
		return ""
	}
	return ": " + review.Status.Reason
}
