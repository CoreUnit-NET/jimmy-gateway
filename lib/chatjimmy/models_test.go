package chatjimmy

import (
	"slices"
	"testing"
)

var sdkAliases = []string{
	"gpt-4o",
	"gpt-4o-mini",
	"gpt-4-turbo",
	"gpt-3.5-turbo",
	"gpt-4",
	"gpt-3.5-turbo-instruct",
	"claude-opus-4-5-20251101",
	"claude-sonnet-4-5-20250929",
	"claude-3-5-haiku-20241022",
	"claude-3-opus-20240229",
	"claude-3-sonnet-20240229",
	"claude-3-haiku-20240307",
	"gemini-1.5-pro",
	"gemini-1.5-flash",
	"gemini-1.0-pro",
	"gemini-1.0-ultra",
}

func TestModelAliasesMatchSDKTables(t *testing.T) {
	if len(ModelAliases) != len(sdkAliases) {
		t.Fatalf("ModelAliases len = %d, want %d", len(ModelAliases), len(sdkAliases))
	}
	for _, alias := range sdkAliases {
		got, ok := ModelAliases[alias]
		if !ok {
			t.Fatalf("missing alias %q", alias)
		}
		if got != DefaultModel {
			t.Fatalf("ModelAliases[%q] = %q, want %q", alias, got, DefaultModel)
		}
	}
}

func TestListedModelsOrder(t *testing.T) {
	got := ListedModels()
	want := append([]string{DefaultModel}, slices.Clone(sdkAliases)...)
	slices.Sort(want[1:])
	if !slices.Equal(got, want) {
		t.Fatalf("ListedModels() = %#v, want %#v", got, want)
	}
}

func TestMapModel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "gpt-4o", want: DefaultModel},
		{in: "  gpt-4o-mini  ", want: DefaultModel},
		{in: "", want: DefaultModel},
		{in: "custom-model", want: "custom-model"},
		{in: DefaultModel, want: DefaultModel},
	}
	for _, tc := range tests {
		if got := MapModel(tc.in); got != tc.want {
			t.Fatalf("MapModel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
