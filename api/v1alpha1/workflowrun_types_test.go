/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package v1alpha1

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func int32Ptr(n int32) *int32 { return &n }
func int64Ptr(n int64) *int64 { return &n }

func TestWorkflowRunJobSpec_Validate(t *testing.T) {
	tests := []struct {
		name     string
		spec     *WorkflowRunJobSpec
		wantErr  bool
		contains []string
	}{
		{
			name:    "nil spec",
			spec:    nil,
			wantErr: false,
		},
		{
			name:    "valid empty spec",
			spec:    &WorkflowRunJobSpec{},
			wantErr: false,
		},
		{
			name: "valid with image and service account",
			spec: &WorkflowRunJobSpec{
				Image:              "myrunner:tag",
				ServiceAccountName: "my-sa",
			},
			wantErr: false,
		},
		{
			name: "invalid backoffLimit negative",
			spec: &WorkflowRunJobSpec{
				BackoffLimit: int32Ptr(-1),
			},
			wantErr:  true,
			contains: []string{"backoffLimit", ">= 0"},
		},
		{
			name: "invalid ttlSecondsAfterFinished negative",
			spec: &WorkflowRunJobSpec{
				TTLSecondsAfterFinished: int32Ptr(-10),
			},
			wantErr:  true,
			contains: []string{"ttlSecondsAfterFinished", ">= 0"},
		},
		{
			name: "invalid activeDeadlineSeconds negative",
			spec: &WorkflowRunJobSpec{
				ActiveDeadlineSeconds: int64Ptr(-5),
			},
			wantErr:  true,
			contains: []string{"activeDeadlineSeconds", ">= 0"},
		},
		{
			name: "invalid serviceAccountName",
			spec: &WorkflowRunJobSpec{
				ServiceAccountName: "invalid name!",
			},
			wantErr:  true,
			contains: []string{"serviceAccountName"},
		},
		{
			name: "volume with empty name",
			spec: &WorkflowRunJobSpec{
				Volumes: []corev1.Volume{{Name: ""}},
			},
			wantErr:  true,
			contains: []string{"volumes[0].name"},
		},
		{
			name: "duplicate volume names",
			spec: &WorkflowRunJobSpec{
				Volumes: []corev1.Volume{{Name: "v1"}, {Name: "v1"}},
			},
			wantErr:  true,
			contains: []string{"duplicate volume name"},
		},
		{
			name: "volumeMount without matching volume",
			spec: &WorkflowRunJobSpec{
				Volumes:      []corev1.Volume{{Name: "v1"}},
				VolumeMounts: []corev1.VolumeMount{{Name: "v2", MountPath: "/data"}},
			},
			wantErr:  true,
			contains: []string{"volumeMounts[0].name", "must refer to a volume"},
		},
		{
			name: "volumeMount with empty mountPath",
			spec: &WorkflowRunJobSpec{
				Volumes:      []corev1.Volume{{Name: "v1"}},
				VolumeMounts: []corev1.VolumeMount{{Name: "v1", MountPath: ""}},
			},
			wantErr:  true,
			contains: []string{"volumeMounts[0].mountPath"},
		},
		{
			name: "env with empty name",
			spec: &WorkflowRunJobSpec{
				Env: []corev1.EnvVar{{Name: "", Value: "x"}},
			},
			wantErr:  true,
			contains: []string{"env[0].name"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && len(tt.contains) > 0 {
				msg := err.Error()
				for _, sub := range tt.contains {
					if !strings.Contains(msg, sub) {
						t.Errorf("Validate() error = %q, want to contain %q", msg, sub)
					}
				}
			}
		})
	}
}
