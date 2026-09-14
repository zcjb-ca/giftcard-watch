package flipp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListAndDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("postal_code") != "K1P1J1" {
			t.Errorf("postal_code = %q", r.URL.Query().Get("postal_code"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/flipp/flyers":
			_, _ = w.Write([]byte(`{
				"flyers":[{
					"id":123,
					"merchant":"Canadian Tire",
					"valid_from":"2026-09-10T00:00:00-04:00",
					"valid_to":"2026-09-17T23:59:59-04:00",
					"path":"flyers/example/",
					"width":4168,
					"height":2560,
					"resolutions":[4,2,1]
				}]
			}`))
		case "/flipp/flyers/123":
			_, _ = w.Write([]byte(`{
				"items":[{"id":9,"name":"Collect $10 CT Money for every $100 spent on Indigo gift cards"}],
				"pages":[{"id":77,"page":2,"left":2084,"bottom":-2560,"right":4168,"top":0}],
				"has_corrections":true
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL+"/flipp", "en-ca", "K1P1J1", 2*time.Second)
	flyers, _, err := client.ListFlyers(context.Background())
	if err != nil {
		t.Fatalf("ListFlyers() error = %v", err)
	}
	if len(flyers) != 1 || flyers[0].ID != "123" || flyers[0].Merchant != "Canadian Tire" {
		t.Fatalf("unexpected flyers: %#v", flyers)
	}
	if flyers[0].TilePath != "flyers/example/" || len(flyers[0].Resolutions) != 3 {
		t.Fatalf("tile metadata = %#v", flyers[0])
	}

	detail, _, err := client.FetchDetail(context.Background(), flyers[0])
	if err != nil {
		t.Fatalf("FetchDetail() error = %v", err)
	}
	if len(detail.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(detail.Items))
	}
	if detail.DeclaredPages != 1 || len(detail.Pages) != 1 {
		t.Fatalf("pages = %#v", detail.Pages)
	}
	page := detail.Pages[0]
	if !page.HasRasterSource() {
		t.Fatalf("page has no raster source: %#v", page)
	}
	if page.TileBaseURL != "https://f.wishabi.net/flyers/example/" {
		t.Fatalf("TileBaseURL = %q", page.TileBaseURL)
	}
	if page.ResolutionIndex != 1 || page.Resolution != 2 {
		t.Fatalf("resolution = index %d value %v", page.ResolutionIndex, page.Resolution)
	}
	if page.CanvasBottom != -2560 {
		t.Fatalf("CanvasBottom = %d", page.CanvasBottom)
	}
	if !detail.HasCorrections {
		t.Fatal("HasCorrections = false")
	}
}
