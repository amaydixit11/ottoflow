/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/client-go/util/jsonpath"
	"k8s.io/klog/v2"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// normalizeJSONPath converts the JSONPath syntax documented on OutputExtraction.Pattern
// ("$.result.recommendations") into the brace-delimited template form that
// k8s.io/client-go/util/jsonpath actually evaluates ("{$.result.recommendations}").
//
// This bridge is load-bearing, not cosmetic: client-go's parser accepts unbraced input
// without error and returns it verbatim as a literal string. Passing the documented form
// through unwrapped therefore yields the pattern text as the extracted "value" instead of
// failing, which is harder to diagnose than no extraction at all.
//
// Callers pass an already-trimmed pattern (see Extract).
func normalizeJSONPath(pattern string) string {
	if strings.HasPrefix(pattern, "{") && strings.HasSuffix(pattern, "}") {
		return pattern
	}
	return "{" + pattern + "}"
}

// DefaultOutputExtractor implements OutputExtractor
type DefaultOutputExtractor struct{}

// NewDefaultOutputExtractor creates a new output extractor
func NewDefaultOutputExtractor() *DefaultOutputExtractor {
	return &DefaultOutputExtractor{}
}

// Extract extracts outputs from agent response according to extraction config
func (e *DefaultOutputExtractor) Extract(response string, config *ottoflowv1alpha1.OutputExtraction) (map[string]interface{}, error) {
	if config == nil {
		// No extraction config - return entire response as "result"
		return map[string]interface{}{
			"result": response,
		}, nil
	}

	extractionType := config.Type
	if extractionType == "" {
		extractionType = "json" // Default
	}

	// Normalise the pattern once here so every branch below can treat "" as "no pattern"
	// without repeating the check. YAML block scalars readily introduce trailing
	// whitespace, and a whitespace-only pattern means the same thing as an absent one.
	pattern := strings.TrimSpace(config.Pattern)

	switch extractionType {
	case "json":
		return e.extractJSON(response, pattern)
	case "regex":
		return e.extractRegex(response, pattern)
	case "text":
		return e.extractText(response, pattern)
	default:
		return nil, fmt.Errorf("unknown extraction type: %s", extractionType)
	}
}

// extractJSON extracts JSON from response using JSON path pattern
func (e *DefaultOutputExtractor) extractJSON(response string, pattern string) (map[string]interface{}, error) {
	// Parse response as JSON
	var jsonData interface{}
	if err := json.Unmarshal([]byte(response), &jsonData); err != nil {
		// Try to extract JSON from text
		jsonRegex := regexp.MustCompile(`(?s)\{.*\}`)
		matches := jsonRegex.FindString(response)
		if matches == "" {
			return nil, fmt.Errorf("no JSON found in response")
		}
		if err := json.Unmarshal([]byte(matches), &jsonData); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	}

	// If a pattern is specified, narrow to that JSONPath.
	if pattern != "" {
		selected, err := selectJSONPath(jsonData, pattern)
		if err != nil {
			return nil, err
		}
		jsonData = selected
	}

	// Return entire JSON object
	if jsonMap, ok := jsonData.(map[string]interface{}); ok {
		return jsonMap, nil
	}

	return map[string]interface{}{
		"result": jsonData,
	}, nil
}

// selectJSONPath evaluates pattern against data and returns the selected value. A pattern
// matching nothing is an error rather than an empty result: silently yielding nothing is
// what made the previous no-op implementation so hard to notice.
func selectJSONPath(data interface{}, pattern string) (interface{}, error) {
	jp := jsonpath.New("outputExtraction").AllowMissingKeys(false)
	if err := jp.Parse(normalizeJSONPath(pattern)); err != nil {
		return nil, fmt.Errorf("invalid JSONPath pattern %q: %w", pattern, err)
	}

	groups, err := jp.FindResults(data)
	if err != nil {
		return nil, fmt.Errorf("JSONPath pattern %q matched nothing: %w", pattern, err)
	}

	var values []interface{}
	for _, group := range groups {
		for _, v := range group {
			values = append(values, v.Interface())
		}
	}

	switch len(values) {
	case 0:
		return nil, fmt.Errorf("JSONPath pattern %q matched nothing", pattern)
	case 1:
		return values[0], nil
	default:
		// Multiple matches (e.g. a wildcard) collapse to a list.
		return values, nil
	}
}

// extractRegex extracts data using regex pattern
func (e *DefaultOutputExtractor) extractRegex(response string, pattern string) (map[string]interface{}, error) {
	if pattern == "" {
		return nil, fmt.Errorf("regex pattern is required")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	matches := re.FindStringSubmatch(response)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no matches found for pattern")
	}

	result := make(map[string]interface{})
	if len(matches) > 1 {
		// First match is full match, rest are capture groups
		for i, match := range matches[1:] {
			result[fmt.Sprintf("group%d", i+1)] = match
		}
	} else {
		result["match"] = matches[0]
	}

	return result, nil
}

// extractText returns the response verbatim. Pattern has no defined meaning for this type
// (use type: regex to select a substring), so it is logged and ignored rather than silently
// dropped. Ignoring beats erroring here: a pattern on type: text has always been a no-op,
// so rejecting it now would break Agents that already carry one.
func (e *DefaultOutputExtractor) extractText(response string, pattern string) (map[string]interface{}, error) {
	if pattern != "" {
		klog.V(1).InfoS("outputExtraction: pattern is ignored for type text; use type regex to select a substring",
			"pattern", pattern)
	}

	return map[string]interface{}{
		"result": response,
	}, nil
}
