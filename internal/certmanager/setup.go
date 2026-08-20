/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package certmanager

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/kyverno/pkg/certmanager"
	tlsMgr "github.com/kyverno/pkg/tls"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Setup creates and starts the agent-executor TLS certificate controller.
// It uses kyverno/pkg to generate self-signed CA and TLS certs (no cert-manager required).
// The controller creates secrets in the given namespace for the agent-executor service.
func Setup(ctx context.Context, logger logr.Logger, config *rest.Config, namespace, serviceName string) error {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	tlsConfig := &tlsMgr.Config{
		ServiceName: serviceName,
		Namespace:   namespace,
	}

	secretClient := clientset.CoreV1().Secrets(namespace)
	renewer := tlsMgr.NewCertRenewer(
		logger,
		secretClient,
		tlsMgr.CertRenewalInterval,
		tlsMgr.CAValidityDuration,
		tlsMgr.TLSValidityDuration,
		"", // server - empty for in-cluster
		tlsConfig,
	)

	// Create informers for secrets in the namespace (filtered by our secret names)
	informerFactory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		12*time.Hour,
		informers.WithNamespace(namespace),
	)
	secretInformer := informerFactory.Core().V1().Secrets()

	ctrl := certmanager.NewController(
		logger,
		secretInformer,
		secretInformer,
		renewer,
		tlsConfig,
	)

	// Start informers
	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	// Run controller (blocking until ctx cancelled)
	go ctrl.Run(ctx, certmanager.Workers)

	logger.Info("agent-executor TLS certificate controller started",
		"namespace", namespace,
		"serviceName", serviceName,
		"caSecret", tlsMgr.GenerateRootCASecretName(tlsConfig),
		"tlsSecret", tlsMgr.GenerateTLSPairSecretName(tlsConfig),
	)
	return nil
}

// GetTLSPairSecretName returns the secret name used for the TLS cert (for mounting in agent-executor).
func GetTLSPairSecretName(serviceName, namespace string) string {
	return tlsMgr.GenerateTLSPairSecretName(&tlsMgr.Config{
		ServiceName: serviceName,
		Namespace:   namespace,
	})
}

// GetRootCASecretName returns the secret name used for the CA cert.
func GetRootCASecretName(serviceName, namespace string) string {
	return tlsMgr.GenerateRootCASecretName(&tlsMgr.Config{
		ServiceName: serviceName,
		Namespace:   namespace,
	})
}
