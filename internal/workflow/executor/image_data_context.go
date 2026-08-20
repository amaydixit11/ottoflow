/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	kyvernoimagedata "github.com/kyverno/sdk/extensions/cel/libs/imagedata"
	kyvernoimagedataloader "github.com/kyverno/sdk/extensions/imagedataloader"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// imageDataContext implements Kyverno's imageData.ContextInterface using an ImageDataFetcher.
type imageDataContext struct {
	ctx     context.Context
	fetcher ImageDataFetcher
}

// NewImageDataContext creates a new imageData context implementation.
// If fetcher is nil, a default implementation backed by the Kyverno SDK loader is used.
// k8sClient and namespace are carried through to that default fetcher but are currently
// inert: the SDK loader ignores the secret lister it is given and registry auth must be
// passed as remote options instead, which OttoFlow does not yet supply. They are retained
// as the plumbing point for when image-pull-secret auth is wired up.
func NewImageDataContext(ctx context.Context, k8sClient client.Client, namespace string, fetcher ImageDataFetcher) (kyvernoimagedata.ContextInterface, error) {
	if fetcher == nil {
		var err error
		fetcher, err = NewDefaultImageDataFetcher(k8sClient, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to create default imageData fetcher: %w", err)
		}
	}
	return &imageDataContext{ctx: ctx, fetcher: fetcher}, nil
}

// GetImageData fetches image metadata via the configured ImageDataFetcher.
func (c *imageDataContext) GetImageData(imageRef string, _ []remote.Option) (map[string]any, error) {
	return c.fetcher.FetchImageData(c.ctx, imageRef)
}

// defaultImageDataFetcher uses the Kyverno SDK imageData loader for anonymous registry
// access, or an optional ImageDataLoader in tests. client and namespace are stored but not
// yet read — see NewImageDataContext.
type defaultImageDataFetcher struct {
	client    client.Client
	namespace string
	loader    ImageDataLoader // optional; when nil, Kyverno loader is used
}

// NewDefaultImageDataFetcher returns an ImageDataFetcher that uses Kyverno's loader.
func NewDefaultImageDataFetcher(k8sClient client.Client, namespace string) (ImageDataFetcher, error) {
	return &defaultImageDataFetcher{client: k8sClient, namespace: namespace}, nil
}

// NewDefaultImageDataFetcherWithLoader returns an ImageDataFetcher that uses the given loader (for tests).
// When loader is nil, behavior is the same as NewDefaultImageDataFetcher.
func NewDefaultImageDataFetcherWithLoader(k8sClient client.Client, namespace string, loader ImageDataLoader) (ImageDataFetcher, error) {
	return &defaultImageDataFetcher{client: k8sClient, namespace: namespace, loader: loader}, nil
}

// FetchImageData implements ImageDataFetcher using Kyverno's fetcher or the injected loader.
func (f *defaultImageDataFetcher) FetchImageData(ctx context.Context, imageRef string) (map[string]any, error) {
	var data interface{}
	if f.loader != nil {
		var err error
		data, err = f.loader.Load(ctx, imageRef)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch image data for %s: %w", imageRef, err)
		}
	} else {
		// The SDK loader stores the secret lister passed to New but never reads it
		// (imagedatafetcher in kyverno/sdk extensions/imagedataloader); registry auth must be
		// supplied via the authOpts argument instead. nil auth options => authn.Anonymous,
		// matching the previous behaviour (OttoFlow passed no imagePullSecrets).
		kyvernoFetcher, err := kyvernoimagedataloader.New(nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create imageData loader: %w", err)
		}
		imageData, err := kyvernoFetcher.FetchImageData(ctx, imageRef, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch image data for %s: %w", imageRef, err)
		}
		data = imageData.Data()
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal image data: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal image data: %w", err)
	}
	return result, nil
}
