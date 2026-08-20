/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// unstructuredAdapter unboxes typed Kubernetes objects into their plain
// map form before delegating to the environment's composed adapter.
// cel-go 0.31 removed the arbitrary-struct wrapping fallback from the
// NativeTypes provider, so a bound *unstructured.Unstructured no longer
// auto-converts and yields "unsupported conversion to ref.Val" instead.
// Known limit: a typed Unstructured nested inside a container that is
// converted wholesale (e.g. []any{u1,u2}) is not unboxed by this adapter;
// no current code path binds such a shape.
type unstructuredAdapter struct {
	base types.Adapter
}

func (a unstructuredAdapter) NativeToValue(value any) ref.Val {
	switch v := value.(type) {
	case unstructured.Unstructured:
		return a.base.NativeToValue(v.Object)
	case *unstructured.Unstructured:
		if v == nil {
			return types.NullValue // explicit: a typed nil ptr is NOT a nil interface
		}
		return a.base.NativeToValue(v.Object)
	default:
		return a.base.NativeToValue(value)
	}
}
