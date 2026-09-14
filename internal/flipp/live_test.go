//go:build integration

package flipp

import (
	"context"
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
		"https://dam.flippenterprise.net/api/flipp",
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
	for _, flyer := range flyers[:limit] {
		detail, _, err := client.FetchDetail(ctx, flyer)
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
			failures = append(failures, fmt.Sprintf("%s: no usable page raster source", flyer.ID))
			continue
		}
		t.Logf("verified flyer %s (%s): %d items, %d pages, tile path present=%v",
			flyer.ID, flyer.Merchant, len(detail.Items), detail.DeclaredPages, flyer.TilePath != "")
		return
	}
	t.Fatalf("no sampled flyer satisfied the live contract; failures: %v", failures)
}
