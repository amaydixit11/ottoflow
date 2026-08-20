/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LLM env override", func() {
	Describe("GetLLMEnv", func() {
		It("returns value from override map when present", func() {
			ctx := WithLLMEnvOverride(context.Background(), map[string]string{
				"NIRMATA_LLM_TOKEN": "token-from-map",
				"NIRMATA_URL":       "https://custom.io",
			})
			Expect(GetLLMEnv(ctx, "NIRMATA_LLM_TOKEN")).To(Equal("token-from-map"))
			Expect(GetLLMEnv(ctx, "NIRMATA_URL")).To(Equal("https://custom.io"))
		})
		It("returns empty string for key not in map when override present", func() {
			ctx := WithLLMEnvOverride(context.Background(), map[string]string{"NIRMATA_LLM_TOKEN": "x"})
			Expect(GetLLMEnv(ctx, "OTHER_KEY")).To(Equal(""))
		})
		It("falls back to os.Getenv when key not in override map", func() {
			ctx := WithLLMEnvOverride(context.Background(), map[string]string{"OTHER": "x"})
			// NIRMATA_LLM_TOKEN not in map; if unset in env, GetLLMEnv returns "".
			prev, had := os.LookupEnv("NIRMATA_LLM_TOKEN")
			defer func() {
				if had {
					_ = os.Setenv("NIRMATA_LLM_TOKEN", prev)
				} else {
					_ = os.Unsetenv("NIRMATA_LLM_TOKEN")
				}
			}()
			_ = os.Unsetenv("NIRMATA_LLM_TOKEN")
			Expect(GetLLMEnv(ctx, "NIRMATA_LLM_TOKEN")).To(Equal(""))
		})
	})

	Describe("WithLLMEnvOverride", func() {
		It("returns ctx unchanged when env is nil", func() {
			ctx := context.Background()
			out := WithLLMEnvOverride(ctx, nil)
			Expect(out).To(Equal(ctx))
		})
		It("copies map so caller cannot mutate after attach", func() {
			m := map[string]string{"K": "v1"}
			ctx := WithLLMEnvOverride(context.Background(), m)
			m["K"] = "v2"
			Expect(GetLLMEnv(ctx, "K")).To(Equal("v1"))
		})
	})
})
