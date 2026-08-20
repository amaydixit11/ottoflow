/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cluster

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Default kubeconfig keys to try in Secret.Data when Key is not specified.
var defaultKubeConfigKeys = []string{"config", "kubeconfig", "value"}

// ClientFromKubeConfigSecret loads a Secret containing a kubeconfig and returns a controller-runtime
// Client for that cluster. secretNamespace and secretName identify the Secret; dataKey is the key in
// Secret.Data holding the kubeconfig bytes. If dataKey is empty, keys "config", "kubeconfig", and
// "value" are tried in order. scheme must be the same as the manager's scheme so all API types are
// registered.
func ClientFromKubeConfigSecret(
	ctx context.Context,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	secretNamespace, secretName, dataKey string,
) (client.Client, error) {
	restConfig, err := RestConfigFromKubeConfigSecret(ctx, k8sClient, secretNamespace, secretName, dataKey)
	if err != nil {
		return nil, err
	}
	return ClientFromRESTConfig(restConfig, scheme)
}

// restConfigFromKubeConfig parses kubeconfig bytes and returns a rest.Config.
func restConfigFromKubeConfig(kubeconfig []byte) (*rest.Config, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}
