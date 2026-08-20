/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

func TestOutputExtractor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OutputExtractor Suite")
}

var _ = Describe("OutputExtractor", func() {
	var extractor *DefaultOutputExtractor

	BeforeEach(func() {
		extractor = NewDefaultOutputExtractor()
	})

	Describe("Extract", func() {
		It("should return entire response as result when config is nil", func() {
			result, err := extractor.Extract("hello world", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("result"))
			Expect(result["result"]).To(Equal("hello world"))
		})

		It("should return error for unknown extraction type", func() {
			config := &ottoflowv1alpha1.OutputExtraction{Type: "unknown"}
			_, err := extractor.Extract("x", config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown extraction type"))
		})

		It("should extract JSON output", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "$.result",
			}

			response := `{"result": "test-value", "other": "ignored"}`
			result, err := extractor.Extract(response, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result).To(HaveKey("result"))
			Expect(result["result"]).To(Equal("test-value"))
		})

		It("should extract nested JSON values", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "$.data.value",
			}

			response := `{"data": {"value": "nested-value"}}`
			result, err := extractor.Extract(response, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// The pattern selects a scalar, so it is returned under "result".
			Expect(result).To(HaveKeyWithValue("result", "nested-value"))
			Expect(result).NotTo(HaveKey("data"))
		})

		It("should return the selected object when the JSON path selects a map", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "$.data",
			}

			response := `{"data": {"value": "nested-value"}, "other": "ignored"}`
			result, err := extractor.Extract(response, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("value", "nested-value"))
			Expect(result).NotTo(HaveKey("other"))
		})

		It("should accept the brace-delimited JSONPath form as well", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "{.data.value}",
			}

			result, err := extractor.Extract(`{"data": {"value": "nested-value"}}`, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("result", "nested-value"))
		})

		It("should extract using regex pattern", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "regex",
				Pattern: `Result: (\w+)`,
			}

			response := "The result is: Result: success"
			result, err := extractor.Extract(response, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result).To(HaveKey("group1"))
			Expect(result["group1"]).To(Equal("success"))
		})

		It("should extract text output", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type: "text",
			}

			response := "Simple text response"
			result, err := extractor.Extract(response, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result).To(HaveKey("result"))
			Expect(result["result"]).To(Equal("Simple text response"))
		})

		It("should extract text with pattern (pattern reserved for future use)", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "text",
				Pattern: "prefix:",
			}
			response := "prefix: value"
			result, err := extractor.Extract(response, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result["result"]).To(Equal("prefix: value"))
		})

		It("should return full JSON when pattern is $", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "$",
			}

			response := `{"key1": "value1", "key2": "value2"}`
			result, err := extractor.Extract(response, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Should return the full JSON object
			Expect(result).To(HaveKey("key1"))
			Expect(result).To(HaveKey("key2"))
			Expect(result["key1"]).To(Equal("value1"))
			Expect(result["key2"]).To(Equal("value2"))
		})

		It("should handle invalid JSON gracefully", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "$.result",
			}

			response := `{invalid json}`
			_, err := extractor.Extract(response, config)
			Expect(err).To(HaveOccurred())
		})

		It("should return error when no JSON found in response", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "",
			}
			_, err := extractor.Extract("plain text with no json", config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no JSON found"))
		})

		It("should fail when the JSON path matches nothing", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "$.nonexistent",
			}

			// A pattern that matches nothing is an error, not a silent fallback to the
			// full object: returning the unfiltered response would let a typo'd path look
			// like a successful extraction.
			_, err := extractor.Extract(`{"result": "value"}`, config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("matched nothing"))
		})

		It("should reject a malformed JSONPath pattern", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "json",
				Pattern: "{.unclosed",
			}

			_, err := extractor.Extract(`{"result": "value"}`, config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid JSONPath pattern"))
		})

		It("should handle regex with no match", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "regex",
				Pattern: `Result: (\w+)`,
			}

			response := "No match here"
			_, err := extractor.Extract(response, config)
			Expect(err).To(HaveOccurred())
		})

		It("should default to json when config type is empty", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "",
				Pattern: "",
			}
			response := `{"x": 1}`
			result, err := extractor.Extract(response, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("x"))
		})

		It("should return error for empty regex pattern", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "regex",
				Pattern: "",
			}
			_, err := extractor.Extract("text", config)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for invalid regex pattern", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "regex",
				Pattern: `[invalid`,
			}
			_, err := extractor.Extract("text", config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid regex"))
		})

		It("should return single match as group when regex has one capture", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "regex",
				Pattern: `(\d+)`,
			}
			result, err := extractor.Extract("count: 42", config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("group1"))
			Expect(result["group1"]).To(Equal("42"))
		})

		It("should return full match as match key when regex has no capture groups", func() {
			config := &ottoflowv1alpha1.OutputExtraction{
				Type:    "regex",
				Pattern: `\d+`,
			}
			result, err := extractor.Extract("count: 42", config)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("match"))
			Expect(result["match"]).To(Equal("42"))
		})
	})
})
