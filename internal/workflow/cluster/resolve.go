/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cluster

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// ClientFromRESTConfig creates a controller-runtime client from a rest.Config.
func ClientFromRESTConfig(restConfig *rest.Config, scheme *runtime.Scheme) (client.Client, error) {
	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create client from rest config: %w", err)
	}
	return c, nil
}

// RestConfigForClusterRef resolves the target cluster config for a WorkflowRun.
// When ClusterRef is nil or local is selected, in-cluster config is used.
func RestConfigForClusterRef(
	ctx context.Context,
	controlClient client.Client,
	workflowRun *ottoflowv1alpha1.WorkflowRun,
) (*rest.Config, error) {
	if workflowRun == nil || workflowRun.Spec.ClusterRef == nil {
		return rest.InClusterConfig()
	}

	clusterRef := workflowRun.Spec.ClusterRef
	if clusterRef.Local != nil && *clusterRef.Local {
		return rest.InClusterConfig()
	}
	if clusterRef.KubeConfigFilePath != "" {
		return RestConfigFromKubeConfigFile(clusterRef.KubeConfigFilePath)
	}
	if clusterRef.KubeConfigSecretRef != nil {
		ref := clusterRef.KubeConfigSecretRef
		secretNamespace := ref.Namespace
		if secretNamespace == "" {
			secretNamespace = workflowRun.Namespace
		}
		return RestConfigFromKubeConfigSecret(ctx, controlClient, secretNamespace, ref.Name, ref.Key)
	}

	return rest.InClusterConfig()
}

// ClientForClusterRef resolves a cluster ref and builds a controller-runtime client for it.
func ClientForClusterRef(
	ctx context.Context,
	controlClient client.Client,
	scheme *runtime.Scheme,
	workflowRun *ottoflowv1alpha1.WorkflowRun,
) (client.Client, error) {
	restConfig, err := RestConfigForClusterRef(ctx, controlClient, workflowRun)
	if err != nil {
		return nil, err
	}
	return ClientFromRESTConfig(restConfig, scheme)
}

// RestConfigFromKubeConfigSecret loads kubeconfig bytes from a Secret and returns a rest.Config.
func RestConfigFromKubeConfigSecret(
	ctx context.Context,
	k8sClient client.Client,
	secretNamespace, secretName, dataKey string,
) (*rest.Config, error) {
	kubeconfig, err := kubeConfigBytesFromSecret(ctx, k8sClient, secretNamespace, secretName, dataKey)
	if err != nil {
		return nil, err
	}
	restConfig, err := restConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build rest config from kubeconfig secret %s/%s: %w", secretNamespace, secretName, err)
	}
	return restConfig, nil
}

// RestConfigFromKubeConfigFile loads kubeconfig bytes from a mounted file and returns a rest.Config.
func RestConfigFromKubeConfigFile(path string) (*rest.Config, error) {
	kubeconfig, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig file %s: %w", path, err)
	}
	restConfig, err := restConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build rest config from kubeconfig file %s: %w", path, err)
	}
	return restConfig, nil
}

func kubeConfigBytesFromSecret(
	ctx context.Context,
	k8sClient client.Client,
	secretNamespace, secretName, dataKey string,
) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: secretNamespace, Name: secretName}, secret); err != nil {
		return nil, fmt.Errorf("get kubeconfig secret %s/%s: %w", secretNamespace, secretName, err)
	}

	if dataKey != "" {
		if b, ok := secret.Data[dataKey]; ok {
			return b, nil
		}
		return nil, fmt.Errorf("secret %s/%s has no key %q", secretNamespace, secretName, dataKey)
	}

	for _, key := range defaultKubeConfigKeys {
		if b, ok := secret.Data[key]; ok {
			return b, nil
		}
	}

	return nil, fmt.Errorf("secret %s/%s has none of keys %v", secretNamespace, secretName, defaultKubeConfigKeys)
}
