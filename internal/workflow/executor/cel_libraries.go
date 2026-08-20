/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	celapi "github.com/google/cel-go/cel"
	celext "github.com/google/cel-go/ext"
	kyvernoglobalcontext "github.com/kyverno/sdk/extensions/cel/libs/globalcontext"
	kyvernohash "github.com/kyverno/sdk/extensions/cel/libs/hash"
	kyvernohttp "github.com/kyverno/sdk/extensions/cel/libs/http"
	kyvernoimage "github.com/kyverno/sdk/extensions/cel/libs/image"
	kyvernoimagedata "github.com/kyverno/sdk/extensions/cel/libs/imagedata"
	kyvernojson "github.com/kyverno/sdk/extensions/cel/libs/json"
	kyvernomath "github.com/kyverno/sdk/extensions/cel/libs/math"
	kyvernorandom "github.com/kyverno/sdk/extensions/cel/libs/random"
	kyvernoresource "github.com/kyverno/sdk/extensions/cel/libs/resource"
	kyvernotime "github.com/kyverno/sdk/extensions/cel/libs/time"
	kyvernotransform "github.com/kyverno/sdk/extensions/cel/libs/transform"
	kyvernouser "github.com/kyverno/sdk/extensions/cel/libs/user"
	kyvernox509 "github.com/kyverno/sdk/extensions/cel/libs/x509"
	kyvernoyaml "github.com/kyverno/sdk/extensions/cel/libs/yaml"
	k8scellib "k8s.io/apiserver/pkg/cel/library"
)

// GetKyvernoCELOptions returns CEL environment options for all Kyverno CEL libraries
// This is a convenience function that creates default implementations for JSON and YAML
func GetKyvernoCELOptions(namespace string) []celapi.EnvOption {
	jsonImpl := &kyvernojson.JsonImpl{}
	yamlImpl := &kyvernoyaml.YamlImpl{}
	return GetKyvernoCELOptionsWithImpls(namespace, jsonImpl, yamlImpl)
}

// noopGlobalContext implements globalcontext.ContextInterface; returns nil for all lookups (used when no global context store is configured).
type noopGlobalContext struct{}

func (noopGlobalContext) GetGlobalReference(_, _ string) (any, error) { return nil, nil }

// GetKyvernoCELOptionsWithImpls returns CEL environment options for all Kyverno CEL libraries (from kyverno/sdk/cel).
// Resource, globalContext, and image contexts are provided at evaluation time via vars; placeholder nils are used at env build.
func GetKyvernoCELOptionsWithImpls(namespace string, jsonImpl kyvernojson.JsonIface, yamlImpl kyvernoyaml.YamlIface) []celapi.EnvOption {
	opts := make([]celapi.EnvOption, 0, 15)

	// Resource library - context provided at evaluation time in vars
	opts = append(opts, kyvernoresource.Lib(nil, "", kyvernoresource.Latest()))

	// HTTP library - use shared HTTP context (same one used at eval)
	opts = append(opts, kyvernohttp.Lib(NewCELHTTPContext(), kyvernohttp.Latest()))

	// User library
	opts = append(opts, kyvernouser.Lib(kyvernouser.Latest()))

	// Image library
	opts = append(opts, kyvernoimage.Lib(kyvernoimage.Latest()))

	// ImageData library - context provided at evaluation time in vars; third arg is
	// registry auth options, left nil since OttoFlow supplies none.
	opts = append(opts, kyvernoimagedata.Lib(nil, kyvernoimagedata.Latest(), nil))

	// GlobalContext library - no-op implementation (no global context store in workflows)
	opts = append(opts, kyvernoglobalcontext.Lib(noopGlobalContext{}, kyvernoglobalcontext.Latest()))

	// Hash library
	opts = append(opts, kyvernohash.Lib(kyvernohash.Latest()))

	// Math library
	opts = append(opts, kyvernomath.Lib(kyvernomath.Latest()))

	// Random library
	opts = append(opts, kyvernorandom.Lib(kyvernorandom.Latest()))

	// Transform library
	opts = append(opts, kyvernotransform.Lib(kyvernotransform.Latest()))

	// JSON library - requires JsonIface implementation
	opts = append(opts, kyvernojson.Lib(jsonImpl, kyvernojson.Latest()))

	// YAML library - requires YamlIface implementation
	opts = append(opts, kyvernoyaml.Lib(yamlImpl, kyvernoyaml.Latest()))

	// Time library (time.now(), time.toCron(), time.truncate(), etc.)
	opts = append(opts, kyvernotime.Lib(kyvernotime.Latest()))

	// X509 library
	opts = append(opts, kyvernox509.Lib(kyvernox509.Latest()))

	return opts
}

// GetKyvernoCELProgramOptions returns ProgramOptions from all Kyverno CEL libraries
// These ProgramOptions must be passed when creating CEL programs to enable library functions
//
// Based on Kyverno's implementation:
// - JSON library's ProgramOptions() returns cel.Globals(map[string]any{"json": l.json})
// - YAML library's ProgramOptions() returns cel.Globals(map[string]any{"yaml": l.yaml})
// - The lib struct stores the Json/Yaml wrapper around the implementation
// - We need to provide these variables at runtime so json.unmarshal() and yaml.parse() work
//
// Since we already have jsonImpl and yamlImpl instances, we can create the ProgramOptions
// directly by wrapping them in Json/Yaml structs (matching Kyverno's pattern).
func GetKyvernoCELProgramOptions(namespace string, jsonImpl kyvernojson.JsonIface, yamlImpl kyvernoyaml.YamlIface) []celapi.ProgramOption {
	programOpts := make([]celapi.ProgramOption, 0, 2)

	// JSON library ProgramOptions: provide json variable
	// This matches what kyvernojson.Lib() does internally in its ProgramOptions() method
	programOpts = append(programOpts, celapi.Globals(map[string]interface{}{
		"json": kyvernojson.Json{JsonIface: jsonImpl},
	}))

	// YAML library ProgramOptions: provide yaml variable
	// This matches what kyvernoyaml.Lib() does internally in its ProgramOptions() method
	programOpts = append(programOpts, celapi.Globals(map[string]interface{}{
		"yaml": kyvernoyaml.Yaml{YamlIface: yamlImpl},
	}))

	return programOpts
}

// GetKubernetesCELOptions returns CEL environment options for Kubernetes CEL libraries
// These libraries provide Kubernetes-specific functionality like list operations, regex,
// URL parsing, IP/CIDR handling, format validation, quantity manipulation, and semver.
func GetKubernetesCELOptions() []celapi.EnvOption {
	opts := make([]celapi.EnvOption, 0, 8)

	// Note: Extended strings library is available through standard CEL functions
	// Kubernetes-specific libraries are added below

	// Kubernetes CEL libraries from k8s.io/apiserver/pkg/cel/library
	// These provide Kubernetes-specific functionality
	opts = append(opts, k8scellib.Lists())     // List operations: indexOf, lastIndexOf, min, max, sum, isSorted
	opts = append(opts, k8scellib.Regex())     // Regex operations: find, findAll
	opts = append(opts, k8scellib.URLs())      // URL parsing: isURL, url().getHost(), etc.
	opts = append(opts, k8scellib.IP())        // IP address operations: isIP, ip().family(), etc.
	opts = append(opts, k8scellib.CIDR())      // CIDR operations: cidr(), isCIDR, containsIP, etc.
	opts = append(opts, k8scellib.Format())    // Format validation: format.dns1123Label(), etc.
	opts = append(opts, k8scellib.Quantity())  // Quantity operations: quantity(), isQuantity, etc.
	opts = append(opts, k8scellib.SemverLib()) // Semver operations: semver(), isSemver, etc.

	// Two-variable comprehensions: transformList, transformMap, transformMapEntry
	// transformMapEntry enables O(n) list-to-map indexing for efficient keyed lookups
	opts = append(opts, celext.TwoVarComprehensions())

	// Note: Authz() and AuthzSelectors() are not included as they require authorizer context
	// which is typically only available in admission control scenarios

	return opts
}
