/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"fmt"
	"sort"

	celapi "github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
)

// GetListFunctions returns CEL environment options for list utility functions.
func GetListFunctions() []celapi.EnvOption {
	// topNByField(list, n, fieldName) -> list
	//
	// Returns up to n elements from list, sorted by fieldName descending.
	// Elements where fieldName is absent or non-numeric are skipped.
	// Stable sort: ties preserve original list order among included elements.
	//
	// This is O(n log n) and avoids the O(n²) nested-filter ranking pattern
	// (list.filter(p, size(list.filter(other, other.field > p.field)) < N))
	// that exceeds CEL's cost budget for large clusters.
	//
	// Example:
	//   topNByField(variables.overProvisionedPods, 20, "savingsUSD")
	topNByFieldFunc := celapi.Function("topNByField",
		celapi.Overload("topNByField_list_int_string",
			[]*celapi.Type{celapi.ListType(celapi.DynType), celapi.IntType, celapi.StringType},
			celapi.ListType(celapi.DynType),
			celapi.FunctionBinding(topNByField),
		),
	)

	// listSample(list, n) -> list
	//
	// Returns the first n elements of a string list, plus a trailing
	// "...and N more" marker when the list has more than n elements. Used to
	// bound how many resource names get embedded in an LLM prompt per
	// category, regardless of how large the raw collected count is — a
	// category with 100 unused resources and a category with 128,000 both
	// produce a prompt contribution of at most n+1 lines.
	//
	// Example:
	//   listSample(variables.unusedPDBs, 25)
	listSampleFunc := celapi.Function("listSample",
		celapi.Overload("listSample_list_int",
			[]*celapi.Type{celapi.ListType(celapi.StringType), celapi.IntType},
			celapi.ListType(celapi.StringType),
			celapi.FunctionBinding(listSample),
		),
	)

	return []celapi.EnvOption{topNByFieldFunc, listSampleFunc}
}

func listSample(args ...ref.Val) ref.Val {
	if len(args) != 2 {
		return types.NewErr("listSample requires exactly 2 arguments: list, n")
	}

	lister, ok := args[0].(traits.Lister)
	if !ok {
		return types.NewErr("listSample: first argument must be a list")
	}

	n, err := celIntArg(args[1])
	if err != nil {
		return types.NewErr("listSample: second argument must be an integer: %v", err)
	}
	if n < 0 {
		n = 0
	}

	listSize, err := celIntArg(lister.Size())
	if err != nil {
		return types.NewErr("listSample: could not determine list size: %v", err)
	}

	limit := n
	if limit > listSize {
		limit = listSize
	}

	result := make([]ref.Val, 0, limit+1)
	for i := 0; i < limit; i++ {
		result = append(result, lister.Get(types.Int(i)))
	}
	if listSize > n {
		result = append(result, types.String(
			fmt.Sprintf("...and %d more", listSize-n)))
	}
	return types.NewDynamicList(types.DefaultTypeAdapter, result)
}

func topNByField(args ...ref.Val) ref.Val {
	if len(args) != 3 {
		return types.NewErr("topNByField requires exactly 3 arguments: list, n, fieldName")
	}

	lister, ok := args[0].(traits.Lister)
	if !ok {
		return types.NewErr("topNByField: first argument must be a list")
	}

	n, err := celIntArg(args[1])
	if err != nil {
		return types.NewErr("topNByField: second argument must be an integer: %v", err)
	}
	if n <= 0 {
		return types.NewDynamicList(types.DefaultTypeAdapter, []ref.Val{})
	}

	fieldName, ok := args[2].(types.String)
	if !ok {
		return types.NewErr("topNByField: third argument must be a string")
	}
	fieldKey := fieldName

	listSize, err := celIntArg(lister.Size())
	if err != nil {
		return types.NewErr("topNByField: could not determine list size: %v", err)
	}

	type entry struct {
		val ref.Val
		key float64
	}

	entries := make([]entry, 0, listSize)
	for i := range listSize {
		elem := lister.Get(types.Int(i))
		if types.IsError(elem) {
			continue
		}
		indexer, ok := elem.(traits.Indexer)
		if !ok {
			continue
		}
		keyVal := indexer.Get(fieldKey)
		if types.IsError(keyVal) {
			continue
		}
		f, ok := toFloat64(keyVal)
		if !ok {
			continue
		}
		entries = append(entries, entry{val: elem, key: f})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].key > entries[j].key
	})

	if n < len(entries) {
		entries = entries[:n]
	}

	result := make([]ref.Val, len(entries))
	for i, e := range entries {
		result[i] = e.val
	}
	return types.NewDynamicList(types.DefaultTypeAdapter, result)
}

// celIntArg converts a ref.Val (types.Int or native int/int64) to int.
func celIntArg(v ref.Val) (int, error) {
	switch tv := v.(type) {
	case types.Int:
		return int(tv), nil
	}
	switch nv := v.Value().(type) {
	case int64:
		return int(nv), nil
	case int:
		return nv, nil
	}
	return 0, fmt.Errorf("expected integer, got %T", v)
}

// toFloat64 converts a numeric ref.Val to float64.
func toFloat64(v ref.Val) (float64, bool) {
	switch tv := v.(type) {
	case types.Double:
		return float64(tv), true
	case types.Int:
		return float64(tv), true
	case types.Uint:
		return float64(tv), true
	}
	switch nv := v.Value().(type) {
	case float64:
		return nv, true
	case int64:
		return float64(nv), true
	case uint64:
		return float64(nv), true
	}
	return 0, false
}
