package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

type fileState struct {
	Seen map[string]time.Time `json:"seen"`
}

func Apply(result *model.Result, statePath string) error {
	state := fileState{Seen: make(map[string]time.Time)}
	if statePath != "" {
		body, err := os.ReadFile(statePath)
		switch {
		case err == nil:
			if err := json.Unmarshal(body, &state); err != nil {
				return errors.New("decode state file: " + err.Error())
			}
		case !errors.Is(err, os.ErrNotExist):
			return errors.New("read state file: " + err.Error())
		}
	}
	if state.Seen == nil {
		state.Seen = make(map[string]time.Time)
	}

	cutoff := result.GeneratedAt.AddDate(0, 0, -120)
	for id, seenAt := range state.Seen {
		if seenAt.Before(cutoff) {
			delete(state.Seen, id)
		}
	}

	result.QualifyingOffers = nil
	result.NewQualifyingOffers = nil
	result.NewReviewCandidates = nil
	for index := range result.Candidates {
		candidate := &result.Candidates[index]
		_, alreadySeen := state.Seen[candidate.ID]
		candidate.IsNew = !alreadySeen
		if candidate.Qualifying {
			result.QualifyingOffers = append(result.QualifyingOffers, *candidate)
			if candidate.IsNew {
				result.NewQualifyingOffers = append(result.NewQualifyingOffers, *candidate)
			}
		}
		if candidate.NeedsReview && candidate.IsNew {
			result.NewReviewCandidates = append(result.NewReviewCandidates, *candidate)
		}
		state.Seen[candidate.ID] = result.GeneratedAt
	}

	if statePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return errors.New("create state directory: " + err.Error())
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return errors.New("encode state file: " + err.Error())
	}
	body = append(body, '\n')
	temporary := statePath + ".tmp"
	if err := os.WriteFile(temporary, body, 0o600); err != nil {
		return errors.New("write state file: " + err.Error())
	}
	if err := os.Rename(temporary, statePath); err != nil {
		return errors.New("replace state file: " + err.Error())
	}
	return nil
}
