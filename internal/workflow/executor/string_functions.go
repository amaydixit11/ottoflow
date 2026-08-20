/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"fmt"

	celapi "github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
)

// GetStringFormatOptions returns CEL environment options for string formatting functions
func GetStringFormatOptions() []celapi.EnvOption {
	opts := make([]celapi.EnvOption, 0, 1)

	// format(formatString, ...args) -> string
	// Similar to Go's fmt.Sprintf, supports format verbs: %s, %d, %v, etc.
	formatFunc := celapi.Function("format",
		celapi.Overload("format_string_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				if len(args) < 1 {
					return types.NewErr("format requires at least 1 argument: format string")
				}

				formatStr, ok := args[0].Value().(string)
				if !ok {
					return types.NewErr("format string must be a string")
				}

				// Convert remaining arguments to interface{} slice for fmt.Sprintf
				formatArgs := make([]interface{}, len(args)-1)
				for i := 1; i < len(args); i++ {
					formatArgs[i-1] = args[i].Value()
				}

				// Use fmt.Sprintf to format the string
				result := fmt.Sprintf(formatStr, formatArgs...)
				return types.String(result)
			}),
		),
		// Variadic overload: format(formatString, arg1, arg2, ...)
		// CEL doesn't have true variadic functions, so we create multiple overloads
		// for common cases (1-19 value arguments, i.e. 2-20 args total)
		celapi.Overload("format_string_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		celapi.Overload("format_string_dyn_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		celapi.Overload("format_string_dyn_dyn_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		celapi.Overload("format_string_dyn_dyn_dyn_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		celapi.Overload("format_string_dyn_dyn_dyn_dyn_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		celapi.Overload("format_string_dyn_dyn_dyn_dyn_dyn_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		celapi.Overload("format_string_dyn_dyn_dyn_dyn_dyn_dyn_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		celapi.Overload("format_string_dyn_dyn_dyn_dyn_dyn_dyn_dyn_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		celapi.Overload("format_string_dyn_dyn_dyn_dyn_dyn_dyn_dyn_dyn_dyn_dyn",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val {
				return formatString(args...)
			}),
		),
		// 12–20 args (for expressions like perNamespaceSummary with many placeholders)
		celapi.Overload("format_string_12",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
		celapi.Overload("format_string_13",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
		celapi.Overload("format_string_14",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
		celapi.Overload("format_string_15",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
		celapi.Overload("format_string_16",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
		celapi.Overload("format_string_17",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
		celapi.Overload("format_string_18",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
		celapi.Overload("format_string_19",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
		celapi.Overload("format_string_20",
			[]*celapi.Type{celapi.StringType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType, celapi.DynType},
			celapi.StringType,
			celapi.FunctionBinding(func(args ...ref.Val) ref.Val { return formatString(args...) })),
	)

	// formatList(formatString, list) -> string
	// Accepts format string and a list of arguments (no variadic cap). Use for 12+ args or dynamic arg count.
	formatListFunc := celapi.Function("formatList",
		celapi.Overload("formatList_string_list",
			[]*celapi.Type{celapi.StringType, celapi.ListType(celapi.DynType)},
			celapi.StringType,
			celapi.FunctionBinding(formatList),
		),
	)

	opts = append(opts, formatFunc, formatListFunc)
	return opts
}

// formatString is a helper function that formats a string using fmt.Sprintf
func formatString(args ...ref.Val) ref.Val {
	if len(args) < 1 {
		return types.NewErr("format requires at least 1 argument: format string")
	}

	formatStr, ok := args[0].Value().(string)
	if !ok {
		return types.NewErr("format string must be a string")
	}

	// Convert remaining arguments to interface{} slice for fmt.Sprintf
	formatArgs := make([]interface{}, len(args)-1)
	for i := 1; i < len(args); i++ {
		formatArgs[i-1] = args[i].Value()
	}

	// Use fmt.Sprintf to format the string
	result := fmt.Sprintf(formatStr, formatArgs...)
	return types.String(result)
}

// formatList implements formatList(formatString, list) for arbitrary-length argument lists.
func formatList(args ...ref.Val) ref.Val {
	if len(args) != 2 {
		return types.NewErr("formatList requires exactly 2 arguments: format string and list of values")
	}
	formatStr, ok := args[0].Value().(string)
	if !ok {
		return types.NewErr("formatList first argument must be a string")
	}
	lister, ok := args[1].(traits.Lister)
	if !ok {
		return types.NewErr("formatList second argument must be a list")
	}
	sizeVal := lister.Size().Value()
	var size int64
	switch s := sizeVal.(type) {
	case int64:
		size = s
	case int:
		size = int64(s)
	default:
		return types.NewErr("formatList list size must be an integer")
	}
	formatArgs := make([]interface{}, size)
	for i := int64(0); i < size; i++ {
		formatArgs[i] = lister.Get(types.Int(i)).Value()
	}
	result := fmt.Sprintf(formatStr, formatArgs...)
	return types.String(result)
}
