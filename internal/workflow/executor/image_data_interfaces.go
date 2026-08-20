/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import "context"

// ImageDataFetcher fetches image metadata for a given image reference.
// Inject a mock in tests to avoid registry/network calls.
type ImageDataFetcher interface {
	FetchImageData(ctx context.Context, imageRef string) (map[string]any, error)
}

// ImageDataLoader loads raw image data (JSON-marshalable). Used inside defaultImageDataFetcher;
// when nil the Kyverno loader is used. Inject a mock in tests to cover FetchImageData and marshal path without registry.
type ImageDataLoader interface {
	Load(ctx context.Context, imageRef string) (data interface{}, err error)
}
