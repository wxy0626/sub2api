package service

import "testing"

func TestResolveAccountTestModelWithDefault(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"flash":    "deepseek-v4-flash",
				"pro-*":    "upstream-pro",
				"passthru": "passthru",
			},
		},
	}

	tests := []struct {
		name    string
		modelID string
		want    string
	}{
		{name: "exact mapping", modelID: "flash", want: "deepseek-v4-flash"},
		{name: "wildcard mapping", modelID: "pro-large", want: "upstream-pro"},
		{name: "unmatched passthrough", modelID: "passthru", want: "passthru"},
		{name: "default mapping", modelID: "", want: "deepseek-v4-flash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAccountTestModelWithDefault(account, tt.modelID, "flash"); got != tt.want {
				t.Fatalf("resolveAccountTestModelWithDefault(%q) = %q, want %q", tt.modelID, got, tt.want)
			}
		})
	}
}
