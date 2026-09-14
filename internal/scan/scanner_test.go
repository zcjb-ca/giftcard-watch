package scan

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFlattenStringsTraversesUnknownFields(t *testing.T) {
	value := map[string]any{
		"name": "Best Buy gift cards",
		"nested": map[string]any{
			"promotion": "Get 10,000 PC Optimum points",
			"amount":    json.Number("100"),
			"url":       "https://example.com/image.jpg",
		},
	}
	got := FlattenStrings(value)
	for _, want := range []string{"Best Buy gift cards", "Get 10,000 PC Optimum points", "100"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FlattenStrings() = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "https://") {
		t.Fatalf("FlattenStrings() included URL: %q", got)
	}
}

func TestMergeOCRDeduplicatesLines(t *testing.T) {
	got := mergeOCR("Gift Cards\n10% OFF\n", "gift cards\nBest Buy\n")
	if got != "Gift Cards\n10% OFF\nBest Buy" {
		t.Fatalf("mergeOCR() = %q", got)
	}
}
