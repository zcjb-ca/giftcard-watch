package state

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

func TestApplyMarksOnlyFirstObservationNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	build := func() model.Result {
		return model.Result{
			GeneratedAt: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
			Candidates: []model.Candidate{
				{ID: "offer-1", Qualifying: true},
				{ID: "review-1", NeedsReview: true},
			},
		}
	}

	first := build()
	if err := Apply(&first, path); err != nil {
		t.Fatal(err)
	}
	if len(first.NewQualifyingOffers) != 1 || len(first.NewReviewCandidates) != 1 {
		t.Fatalf("first result: %#v", first)
	}

	second := build()
	if err := Apply(&second, path); err != nil {
		t.Fatal(err)
	}
	if len(second.NewQualifyingOffers) != 0 || len(second.NewReviewCandidates) != 0 {
		t.Fatalf("second result repeated notifications: %#v", second)
	}
}
