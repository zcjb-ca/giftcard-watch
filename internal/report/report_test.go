package report

import (
	"strings"
	"testing"
	"time"

	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

func TestMarkdownWarnsAgainstFalseNegative(t *testing.T) {
	value := Markdown(model.Result{
		GeneratedAt: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		Complete:    false,
		Coverage: []model.MerchantCoverage{
			{Merchant: "Loblaws", Required: true, Errors: []string{"no current flyer"}},
		},
	})
	if !strings.Contains(value, "do not interpret this as no offers") {
		t.Fatalf("Markdown() did not include incomplete-scan warning:\n%s", value)
	}
}
