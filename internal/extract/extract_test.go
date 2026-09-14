package extract

import (
	"math"
	"testing"

	"github.com/zcjb-ca/giftcard-watch/internal/config"
	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

func TestAnalyzePromotions(t *testing.T) {
	cfg := config.Default()
	cfg.MinimumReturnPercent = 8
	analyzer := New(cfg)
	source := model.Source{Kind: "structured_json", Merchant: "Test", FlyerID: "1"}

	tests := []struct {
		name        string
		text        string
		wantBrand   string
		wantKind    string
		wantReturn  float64
		wantReview  bool
	}{
		{
			name:       "PC Optimum",
			text:       "Get 10,000 PC Optimum points for every $100 spent on Best Buy gift cards",
			wantBrand:  "Best Buy",
			wantKind:   "points",
			wantReturn: 10,
		},
		{
			name:       "Scene Plus",
			text:       "Earn 1,000 Scene+ points when you buy $100 in Apple gift cards",
			wantBrand:  "Apple",
			wantKind:   "points",
			wantReturn: 10,
		},
		{
			name:       "Canadian Tire Money",
			text:       "Collect $10 CT Money for every $100 spent on Indigo gift cards",
			wantBrand:  "Indigo",
			wantKind:   "store_currency",
			wantReturn: 10,
		},
		{
			name:       "direct percent",
			text:       "Save 10% off Best Buy gift cards",
			wantBrand:  "Best Buy",
			wantKind:   "direct_percent",
			wantReturn: 10,
		},
		{
			name:       "discounted face value",
			text:       "$500 Porter eGift Card for $449.99",
			wantBrand:  "Porter",
			wantKind:   "discounted_face_value",
			wantReturn: 10.002,
		},
		{
			name:       "unparsed candidate",
			text:       "Collect bonus points on participating gift cards",
			wantReview: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := analyzer.Analyze(test.text, source)
			if !ok {
				t.Fatal("Analyze() did not return a candidate")
			}
			if got.NeedsReview != test.wantReview {
				t.Fatalf("NeedsReview = %v, want %v; reason=%s", got.NeedsReview, test.wantReview, got.Reason)
			}
			if test.wantReview {
				return
			}
			if len(got.Brands) == 0 || got.Brands[0] != test.wantBrand {
				t.Fatalf("Brands = %#v, want %q", got.Brands, test.wantBrand)
			}
			if got.Reward.Kind != test.wantKind {
				t.Fatalf("Kind = %q, want %q", got.Reward.Kind, test.wantKind)
			}
			if math.Abs(got.Reward.ReturnPercent-test.wantReturn) > 0.01 {
				t.Fatalf("ReturnPercent = %.4f, want %.4f", got.Reward.ReturnPercent, test.wantReturn)
			}
			if !got.Qualifying {
				t.Fatal("Qualifying = false")
			}
		})
	}
}

func TestAnalyzeIgnoresUnrelatedProduct(t *testing.T) {
	analyzer := New(config.Default())
	_, ok := analyzer.Analyze("Save 10% on apples", model.Source{})
	if ok {
		t.Fatal("unrelated grocery product was treated as gift-card candidate")
	}
}
