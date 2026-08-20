/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("formatValueForPrompt", func() {
	It("should return strings as-is without JSON quoting", func() {
		Expect(formatValueForPrompt("hello world")).To(Equal("hello world"))
	})

	It("should return empty string as-is", func() {
		Expect(formatValueForPrompt("")).To(Equal(""))
	})

	It("should serialize a map as JSON", func() {
		input := map[string]interface{}{
			"key": "value",
		}
		result := formatValueForPrompt(input)
		Expect(result).To(Equal(`{"key":"value"}`))
	})

	It("should serialize a slice as JSON array", func() {
		input := []interface{}{"a", "b", "c"}
		result := formatValueForPrompt(input)
		Expect(result).To(Equal(`["a","b","c"]`))
	})

	It("should serialize nested structures as JSON", func() {
		input := map[string]interface{}{
			"violations": []interface{}{
				map[string]interface{}{
					"name":      "pod-1",
					"namespace": "default",
				},
			},
		}
		result := formatValueForPrompt(input)
		Expect(result).To(MatchJSON(`{"violations":[{"name":"pod-1","namespace":"default"}]}`))
	})

	It("should format integers with fmt.Sprintf", func() {
		Expect(formatValueForPrompt(42)).To(Equal("42"))
		Expect(formatValueForPrompt(int64(9999))).To(Equal("9999"))
	})

	It("should format floats with fmt.Sprintf", func() {
		Expect(formatValueForPrompt(3.14)).To(Equal("3.14"))
	})

	It("should format booleans with fmt.Sprintf", func() {
		Expect(formatValueForPrompt(true)).To(Equal("true"))
		Expect(formatValueForPrompt(false)).To(Equal("false"))
	})

	It("should return empty string for nil", func() {
		Expect(formatValueForPrompt(nil)).To(Equal(""))
	})

	It("should serialize map[string]string as JSON", func() {
		input := map[string]string{"app": "nginx", "env": "prod"}
		result := formatValueForPrompt(input)
		Expect(result).To(MatchJSON(`{"app":"nginx","env":"prod"}`))
	})

	It("should serialize []string as JSON array", func() {
		input := []string{"one", "two", "three"}
		result := formatValueForPrompt(input)
		Expect(result).To(Equal(`["one","two","three"]`))
	})
})
