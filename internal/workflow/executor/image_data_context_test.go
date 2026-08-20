/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// MockImageDataFetcher is a test double for ImageDataFetcher.
type MockImageDataFetcher struct {
	Data map[string]any
	Err  error
}

func (m *MockImageDataFetcher) FetchImageData(_ context.Context, _ string) (map[string]any, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Data, nil
}

var _ ImageDataFetcher = (*MockImageDataFetcher)(nil)

// MockImageDataLoader is a test double for ImageDataLoader.
type MockImageDataLoader struct {
	Data interface{}
	Err  error
}

func (m *MockImageDataLoader) Load(_ context.Context, _ string) (interface{}, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Data, nil
}

var _ ImageDataLoader = (*MockImageDataLoader)(nil)

var _ = Describe("ImageDataContext", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("NewImageDataContext succeeds with fake client", func() {
		imgCtx, err := NewImageDataContext(ctx, fakeClient, "default", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(imgCtx).NotTo(BeNil())
	})

	It("GetImageData returns mock data when using MockImageDataFetcher", func() {
		mockData := map[string]any{"digest": "sha256:abc", "registry": "example.com"}
		fetcher := &MockImageDataFetcher{Data: mockData}
		imgCtx, err := NewImageDataContext(ctx, fakeClient, "default", fetcher)
		Expect(err).NotTo(HaveOccurred())
		Expect(imgCtx).NotTo(BeNil())

		data, err := imgCtx.GetImageData("example.com/image:tag", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(mockData))
		Expect(data["digest"]).To(Equal("sha256:abc"))
	})

	It("GetImageData returns error when MockImageDataFetcher returns error", func() {
		fetcher := &MockImageDataFetcher{Err: fmt.Errorf("registry unreachable")}
		imgCtx, err := NewImageDataContext(ctx, fakeClient, "default", fetcher)
		Expect(err).NotTo(HaveOccurred())

		_, err = imgCtx.GetImageData("example.com/image:tag", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("registry unreachable"))
	})
})

var _ = Describe("defaultImageDataFetcher with ImageDataLoader", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("FetchImageData uses loader and marshal path when loader is set", func() {
		raw := map[string]string{"digest": "sha256:xyz", "registry": "reg.io"}
		loader := &MockImageDataLoader{Data: raw}
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		fetcher, err := NewDefaultImageDataFetcherWithLoader(fakeClient, "default", loader)
		Expect(err).NotTo(HaveOccurred())

		result, err := fetcher.FetchImageData(ctx, "reg.io/img:tag")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveKey("digest"))
		Expect(result["digest"]).To(Equal("sha256:xyz"))
		Expect(result["registry"]).To(Equal("reg.io"))
	})

	It("FetchImageData returns error when loader returns error", func() {
		loader := &MockImageDataLoader{Err: fmt.Errorf("load failed")}
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		fetcher, err := NewDefaultImageDataFetcherWithLoader(fakeClient, "default", loader)
		Expect(err).NotTo(HaveOccurred())

		_, err = fetcher.FetchImageData(ctx, "reg.io/img:tag")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("load failed"))
	})
})
