//go:build integration

package flipp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLiveContract(t *testing.T) {
	postalCode := os.Getenv("FLIPP_SMOKE_POSTAL_CODE")
	if postalCode == "" {
		t.Skip("FLIPP_SMOKE_POSTAL_CODE is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := New(
		"https://backflipp.wishabi.com/flipp",
		"en-ca",
		postalCode,
		20*time.Second,
	)

	flyers, _, err := client.ListFlyers(ctx)
	if err != nil {
		t.Fatalf("ListFlyers() live contract failed: %v", err)
	}
	if len(flyers) == 0 {
		t.Fatal("ListFlyers() returned no flyers")
	}

	limit := len(flyers)
	if limit > 12 {
		limit = 12
	}
	failures := make([]string, 0)
	pageSample := ""
	for _, flyer := range flyers[:limit] {
		detail, raw, err := client.FetchDetail(ctx, flyer.ID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", flyer.ID, err))
			continue
		}
		if len(detail.Items) == 0 {
			failures = append(failures, fmt.Sprintf("%s: no items", flyer.ID))
			continue
		}
		if detail.DeclaredPages == 0 {
			failures = append(failures, fmt.Sprintf("%s: no pages", flyer.ID))
			continue
		}
		if detail.DeclaredPages-detail.MissingPageImages == 0 {
			if pageSample == "" {
				var root map[string]any
				if json.Unmarshal(raw, &root) == nil {
					if pages, ok := root["pages"].([]any); ok && len(pages) > 0 {
						if sample, err := json.Marshal(pages[0]); err == nil {
							pageSample = string(sample)
						}
					}
				}
			}
			failures = append(failures, fmt.Sprintf("%s: no usable page image URL", flyer.ID))
			continue
		}
		t.Logf("verified flyer %s (%s): %d items, %d pages",
			flyer.ID, flyer.Merchant, len(detail.Items), detail.DeclaredPages)
		return
	}
	t.Fatalf("no sampled flyer satisfied the live contract; first page sample: %s; failures: %v", pageSample, failures)
}
