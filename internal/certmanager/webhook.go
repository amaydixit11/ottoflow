/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package certmanager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	tlsMgr "github.com/kyverno/pkg/tls"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	webhookCertWaitInterval = 500 * time.Millisecond
	webhookCertWaitTimeout  = 60 * time.Second
)

// BootstrapWebhookCerts starts the internal cert manager for the webhook service, waits for the TLS
// secret to exist, writes tls.crt and tls.key to a temporary cert directory, and returns that
// directory and the CA bundle (PEM) for use in ValidatingWebhookConfiguration clientConfig.caBundle.
// The cert manager continues running in the background for renewal.
func BootstrapWebhookCerts(ctx context.Context, logger logr.Logger, config *rest.Config, namespace, serviceName string) (certDir string, caBundle []byte, err error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", nil, err
	}

	// Start webhook cert manager (creates/renews CA and TLS secrets)
	if err := Setup(ctx, logger, config, namespace, serviceName); err != nil {
		return "", nil, err
	}

	tlsSecretName := tlsMgr.GenerateTLSPairSecretName(&tlsMgr.Config{ServiceName: serviceName, Namespace: namespace})
	caSecretName := tlsMgr.GenerateRootCASecretName(&tlsMgr.Config{ServiceName: serviceName, Namespace: namespace})
	secretClient := clientset.CoreV1().Secrets(namespace)

	// Wait for TLS secret to exist
	deadline := time.Now().Add(webhookCertWaitTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		default:
		}
		tlsSecret, err := secretClient.Get(ctx, tlsSecretName, metav1.GetOptions{})
		if err == nil && tlsSecret != nil && len(tlsSecret.Data[corev1.TLSCertKey]) > 0 && len(tlsSecret.Data[corev1.TLSPrivateKeyKey]) > 0 {
			// Create cert dir and write files
			certDir, err = os.MkdirTemp("", "ottoflow-webhook-certs-")
			if err != nil {
				return "", nil, err
			}
			if err := os.WriteFile(filepath.Join(certDir, "tls.crt"), tlsSecret.Data[corev1.TLSCertKey], 0600); err != nil {
				_ = os.RemoveAll(certDir)
				return "", nil, err
			}
			if err := os.WriteFile(filepath.Join(certDir, "tls.key"), tlsSecret.Data[corev1.TLSPrivateKeyKey], 0600); err != nil {
				_ = os.RemoveAll(certDir)
				return "", nil, err
			}

			// Read CA for VWC caBundle
			caSecret, err := secretClient.Get(ctx, caSecretName, metav1.GetOptions{})
			if err != nil {
				_ = os.RemoveAll(certDir)
				return "", nil, err
			}
			caBundle = caSecret.Data[corev1.TLSCertKey]
			if len(caBundle) == 0 {
				caBundle = caSecret.Data["rootCA.crt"]
			}
			if len(caBundle) == 0 {
				_ = os.RemoveAll(certDir)
				return "", nil, fmt.Errorf("CA secret %s/%s has no tls.crt or rootCA.crt", namespace, caSecretName)
			}
			logger.Info("webhook TLS certs ready", "certDir", certDir, "tlsSecret", tlsSecretName)
			return certDir, caBundle, nil
		}
		time.Sleep(webhookCertWaitInterval)
	}
	return "", nil, fmt.Errorf("timed out waiting for webhook TLS secret %s/%s", namespace, tlsSecretName)
}

// PatchValidatingWebhookConfigCA updates the given ValidatingWebhookConfiguration so that every
// webhook's clientConfig.caBundle is set to the provided CA PEM. No-op if vwcName is empty or the VWC is not found.
func PatchValidatingWebhookConfigCA(ctx context.Context, clientset *kubernetes.Clientset, vwcName string, caBundle []byte) error {
	if vwcName == "" || len(caBundle) == 0 {
		return nil
	}
	adm := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations()
	vwc, err := adm.Get(ctx, vwcName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // chart may not have created it yet
		}
		return err
	}
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.CABundle = caBundle
	}
	_, err = adm.Update(ctx, vwc, metav1.UpdateOptions{})
	return err
}
