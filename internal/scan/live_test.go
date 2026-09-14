//go:build integration

package scan

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/zcjb-ca/giftcard-watch/internal/flipp"
)

func TestLiveTileStitch(t *testing.T) {
	postalCode := os.Getenv("FLIPP_SMOKE_POSTAL_CODE")
	if postalCode == "" {
		t.Skip("FLIPP_SMOKE_POSTAL_CODE is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := flipp.New(
		"https://backflipp.wishabi.com/flipp",
		"https://dam.flippenterprise.net/api/flipp",
		"en-ca",
		postalCode,
		20*time.Second,
	)
	flyers, _, err := client.ListFlyers(ctx)
	if err != nil {
		t.Fatalf("ListFlyers() live tile test failed: %v", err)
	}

	engine := NewOCR("tesseract", "eng", 60*time.Second)
	limit := len(flyers)
	if limit > 4 {
		limit = 4
	}
	failures := make([]string, 0)
	for _, flyer := range flyers[:limit] {
		detail, _, err := client.FetchDetail(ctx, flyer)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s detail: %v", flyer.ID, err))
			continue
		}
		for _, page := range detail.Pages {
			if page.TileBaseURL == "" || !page.HasRasterSource() {
				continue
			}
			rendered, err := engine.renderTiles(ctx, page)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s page %d: %v", flyer.ID, page.Number, err))
				break
			}
			bounds := rendered.Bounds()
			if bounds.Dx() < 500 || bounds.Dy() < 500 {
				failures = append(failures, fmt.Sprintf(
					"%s page %d: stitched image unexpectedly small: %v", flyer.ID, page.Number, bounds,
				))
				break
			}
			_, _, _, alpha := rendered.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2).RGBA()
			if alpha == 0 {
				failures = append(failures, fmt.Sprintf(
					"%s page %d: stitched image center is transparent", flyer.ID, page.Number,
				))
				break
			}
			t.Logf("stitched flyer %s page %d into %dx%d pixels",
				flyer.ID, page.Number, bounds.Dx(), bounds.Dy())
			return
		}
	}
	t.Fatalf("could not stitch any sampled live flyer page: %v", failures)
}
