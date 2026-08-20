/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("parseToolResult", func() {
	It("returns empty string for empty input", func() {
		out, err := parseToolResult("")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(""))
	})

	It("returns original string when trimmed is empty", func() {
		out, err := parseToolResult("  \n\t  ")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("  \n\t  "))
	})

	It("parses JSON object", func() {
		out, err := parseToolResult(`{"a":1,"b":"x"}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeAssignableToTypeOf(map[string]interface{}(nil)))
		m := out.(map[string]interface{})
		Expect(m["a"]).To(BeEquivalentTo(1))
		Expect(m["b"]).To(Equal("x"))
	})

	It("parses JSON array", func() {
		out, err := parseToolResult(`[1,2,"three"]`)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeAssignableToTypeOf([]interface{}(nil)))
		Expect(out.([]interface{})).To(HaveLen(3))
	})

	It("parses number", func() {
		out, err := parseToolResult("42.5")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeEquivalentTo(42.5))
	})

	It("parses boolean", func() {
		out, err := parseToolResult("true")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(true))
	})

	It("returns raw string when not JSON or number or bool", func() {
		out, err := parseToolResult("hello world")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("hello world"))
	})
})

var _ = Describe("buildMCPClientConfig", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		k8sClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(corev1.AddToScheme(scheme))
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("returns error for stdio without command", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "stdio"},
			},
		}
		_, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("command"))
	})

	It("builds stdio config with command and args", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{
					Type:    "stdio",
					Command: []string{"/bin/echo", "-n", "hi"},
				},
			},
		}
		cfg, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Name).To(Equal("s"))
		Expect(cfg.Command).To(Equal("/bin/echo"))
		Expect(cfg.Args).To(Equal([]string{"-n", "hi"}))
		Expect(cfg.Timeout).To(Equal(90))
	})

	It("builds stdio config with timeout", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{
					Type:    "stdio",
					Command: []string{"echo"},
				},
				Timeout: "30s",
			},
		}
		cfg, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Timeout).To(Equal(30))
	})

	It("returns error for http/sse without address", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http"},
			},
		}
		_, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("address"))
	})

	It("builds http config with address and headers", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{
					Type:    "http",
					Address: "https://mcp.example.com",
					Headers: map[string]string{"X-Custom": "val"},
				},
			},
		}
		cfg, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.URL).To(Equal("https://mcp.example.com"))
		Expect(cfg.UseStreaming).To(BeFalse())
		Expect(cfg.Headers).To(Equal(map[string]string{"X-Custom": "val"}))
	})

	It("sets UseStreaming for sse transport", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{
					Type:    "sse",
					Address: "https://mcp.example.com",
				},
			},
		}
		cfg, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.UseStreaming).To(BeTrue())
	})

	It("returns error for unsupported transport type", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "grpc"},
			},
		}
		_, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported"))
	})

	It("builds http config with auth from secret", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"},
			Data:       map[string][]byte{"token": []byte("bearer-token")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://mcp.example.com"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type:      "bearer",
					SecretRef: &ottoflowv1alpha1.SecretReference{Name: "auth", Key: "token"},
				},
			},
		}
		cfg, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth).NotTo(BeNil())
		Expect(cfg.Auth.Token).To(Equal("bearer-token"))
	})

	It("builds stdio config with env from ValueFrom secret", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "env-secret", Namespace: "default"},
			Data:       map[string][]byte{"api_key": []byte("secret-key")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{
					Type:    "stdio",
					Command: []string{"echo"},
				},
				Env: []corev1.EnvVar{
					{Name: "API_KEY", ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "env-secret"},
							Key:                  "api_key",
						},
					}},
				},
			},
		}
		cfg, err := buildMCPClientConfig(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Env).To(ContainElement("API_KEY=secret-key"))
	})
})

var _ = Describe("resolveEnvValue", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		k8sClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		utilruntime.Must(corev1.AddToScheme(scheme))
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("returns Value when set", func() {
		ev := &corev1.EnvVar{Name: "FOO", Value: "bar"}
		Expect(resolveEnvValue(ctx, k8sClient, "default", ev)).To(Equal("bar"))
	})

	It("returns secret value when ValueFrom.SecretKeyRef is set", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
			Data:       map[string][]byte{"token": []byte("secret-val")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		ev := &corev1.EnvVar{
			Name: "TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "s1"}, Key: "token"},
			},
		}
		Expect(resolveEnvValue(ctx, k8sClient, "default", ev)).To(Equal("secret-val"))
	})

	It("returns empty when secret not found", func() {
		ev := &corev1.EnvVar{
			Name: "TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "missing"}, Key: "token"},
			},
		}
		Expect(resolveEnvValue(ctx, k8sClient, "default", ev)).To(Equal(""))
	})
})

var _ = Describe("resolveAuthConfigs", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		k8sClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(corev1.AddToScheme(scheme))
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("returns nil when auth is nil", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec:       ottoflowv1alpha1.MCPServerSpec{Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"}},
		}
		ac, oauth, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(ac).To(BeNil())
		Expect(oauth).To(BeNil())
	})

	It("returns error for bearer auth without secretRef", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth:      &ottoflowv1alpha1.AuthConfig{Type: "bearer"},
			},
		}
		_, _, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("secretRef"))
	})

	It("resolves bearer auth from secret", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"},
			Data:       map[string][]byte{"token": []byte("bearer-token")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type:      "bearer",
					SecretRef: &ottoflowv1alpha1.SecretReference{Name: "auth", Key: "token"},
				},
			},
		}
		ac, oauth, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(oauth).To(BeNil())
		Expect(ac).NotTo(BeNil())
		Expect(ac.Type).To(Equal("bearer"))
		Expect(ac.Token).To(Equal("bearer-token"))
	})

	It("resolves apiKey auth and sets ApiKey", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"},
			Data:       map[string][]byte{"apikey": []byte("my-api-key")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type:      "apiKey",
					SecretRef: &ottoflowv1alpha1.SecretReference{Name: "auth", Key: "apikey"},
				},
			},
		}
		ac, _, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(ac.Type).To(Equal("api-key"))
		Expect(ac.Token).To(Equal("my-api-key"))
		Expect(ac.ApiKey).To(Equal("my-api-key"))
	})

	It("returns error for basic auth without username/password in secret", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"},
			Data:       map[string][]byte{"username": []byte("u")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type:      "basic",
					SecretRef: &ottoflowv1alpha1.SecretReference{Name: "auth", Key: "x"},
				},
			},
		}
		_, _, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("username and password"))
	})

	It("resolves basic auth from secret", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"},
			Data:       map[string][]byte{"username": []byte("u"), "password": []byte("p")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type:      "basic",
					SecretRef: &ottoflowv1alpha1.SecretReference{Name: "auth", Key: "x"},
				},
			},
		}
		ac, _, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(ac.Username).To(Equal("u"))
		Expect(ac.Password).To(Equal("p"))
	})

	It("returns error for oauth2 without config", func() {
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth:      &ottoflowv1alpha1.AuthConfig{Type: "oauth2"},
			},
		}
		_, _, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("oauth2 config"))
	})

	It("resolves oauth2 with ClientCredentialsRef", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "oauth", Namespace: "default"},
			Data:       map[string][]byte{"client_id": []byte("cid"), "client_secret": []byte("csec")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type: "oauth2",
					OAuth2: &ottoflowv1alpha1.OAuth2Config{
						TokenURL:             "https://auth.example.com/token",
						Scopes:               []string{"read"},
						ClientCredentialsRef: &ottoflowv1alpha1.NamespacedSecretRef{Name: "oauth", Namespace: "default"},
					},
				},
			},
		}
		ac, oauth, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(ac).NotTo(BeNil())
		Expect(oauth).NotTo(BeNil())
		Expect(oauth.TokenURL).To(Equal("https://auth.example.com/token"))
		Expect(oauth.Scopes).To(Equal([]string{"read"}))
		Expect(oauth.ClientID).To(Equal("cid"))
		Expect(oauth.ClientSecret).To(Equal("csec"))
	})

	It("resolves oauth2 with ClientID and ClientSecretRef", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "oauth-secret", Namespace: "default"},
			Data:       map[string][]byte{"secret": []byte("my-client-secret")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type: "oauth2",
					OAuth2: &ottoflowv1alpha1.OAuth2Config{
						TokenURL: "https://auth.example.com/token",
						ClientID: "my-client-id",
						ClientSecretRef: &ottoflowv1alpha1.SecretReference{
							Name: "oauth-secret", Key: "secret",
						},
					},
				},
			},
		}
		_, oauth, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(oauth).NotTo(BeNil())
		Expect(oauth.ClientID).To(Equal("my-client-id"))
		Expect(oauth.ClientSecret).To(Equal("my-client-secret"))
	})

	It("returns error when auth secret key not found", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"},
			Data:       map[string][]byte{"other": []byte("x")},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type:      "bearer",
					SecretRef: &ottoflowv1alpha1.SecretReference{Name: "auth", Key: "token"},
				},
			},
		}
		_, _, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found in secret"))
	})

	It("returns error when auth secret get fails", func() {
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "http", Address: "https://x"},
				Auth: &ottoflowv1alpha1.AuthConfig{
					Type:      "bearer",
					SecretRef: &ottoflowv1alpha1.SecretReference{Name: "missing", Key: "token"},
				},
			},
		}
		_, _, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get auth secret"))
	})
})

var _ = Describe("DefaultMCPClientFactory CreateClient", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		k8sClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(corev1.AddToScheme(scheme))
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("returns error for unsupported transport type", func() {
		f := NewDefaultMCPClientFactory(k8sClient)
		mcpServer := &ottoflowv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec: ottoflowv1alpha1.MCPServerSpec{
				Transport: ottoflowv1alpha1.TransportConfig{Type: "grpc"},
			},
		}
		_, err := f.CreateClient(ctx, mcpServer)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported"))
	})
})
