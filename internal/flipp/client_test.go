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
			_, _ = w.Write([]byte(`{"flyers":[{"id":123,"merchant":"Canadian Tire","valid_from":"2026-09-10T00:00:00-04:00","valid_to":"2026-09-17T23:59:59-04:00"}]}`))
		case "/flipp/flyers/123":
			_, _ = w.Write([]byte(`{
				"items":[{"id":9,"name":"Collect $10 CT Money for every $100 spent on Indigo gift cards"}],
				"pages":[{"page_number":1,"thumbnail_url":"https://cdn.example/thumb.jpg","large_image_url":"http://cdn.example/page.jpg"}],
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

	detail, _, err := client.FetchDetail(context.Background(), "123")
	if err != nil {
		t.Fatalf("FetchDetail() error = %v", err)
	}
	if len(detail.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(detail.Items))
	}
	if detail.DeclaredPages != 1 || len(detail.Pages) != 1 {
		t.Fatalf("pages = %#v", detail.Pages)
	}
	if detail.Pages[0].ImageURL != "https://cdn.example/page.jpg" {
		t.Fatalf("ImageURL = %q", detail.Pages[0].ImageURL)
	}
	if !detail.HasCorrections {
		t.Fatal("HasCorrections = false")
	}
}
