package model

import "time"

type Flyer struct {
	ID                string
	Merchant          string
	CanonicalMerchant string
	ValidFrom         time.Time
	ValidTo           time.Time
}

func (f Flyer) IsCurrent(now time.Time) bool {
	if !f.ValidFrom.IsZero() && now.Before(f.ValidFrom) {
		return false
	}
	if !f.ValidTo.IsZero() && now.After(f.ValidTo) {
		return false
	}
	return true
}

type Page struct {
	Number   int    `json:"number"`
	ImageURL string `json:"image_url,omitempty"`
}

type Source struct {
	Kind      string `json:"kind"`
	Merchant  string `json:"merchant"`
	FlyerID   string `json:"flyer_id"`
	Page      int    `json:"page,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
	ValidFrom string `json:"valid_from,omitempty"`
	ValidTo   string `json:"valid_to,omitempty"`
}

type Reward struct {
	Kind          string  `json:"kind,omitempty"`
	Program       string  `json:"program,omitempty"`
	SpendCAD      float64 `json:"spend_cad,omitempty"`
	FaceValueCAD  float64 `json:"face_value_cad,omitempty"`
	CostCAD       float64 `json:"cost_cad,omitempty"`
	Points        float64 `json:"points,omitempty"`
	RewardCAD     float64 `json:"reward_cad,omitempty"`
	ReturnPercent float64 `json:"return_percent,omitempty"`
}

type Candidate struct {
	ID          string   `json:"id"`
	Brands      []string `json:"brands,omitempty"`
	RawText     string   `json:"raw_text"`
	Source      Source   `json:"source"`
	Reward      Reward   `json:"reward,omitempty"`
	Confidence  string   `json:"confidence"`
	NeedsReview bool     `json:"needs_review"`
	Reason      string   `json:"reason,omitempty"`
	Qualifying  bool     `json:"qualifying"`
	IsNew       bool     `json:"is_new"`
}

type MerchantCoverage struct {
	Merchant          string   `json:"merchant"`
	Required          bool     `json:"required"`
	FlyersFound       int      `json:"flyers_found"`
	DetailsFetched    int      `json:"details_fetched"`
	ItemsScanned      int      `json:"items_scanned"`
	PagesDeclared     int      `json:"pages_declared"`
	PageImagesFound   int      `json:"page_images_found"`
	PagesOCRed        int      `json:"pages_ocr_ed"`
	CorrectionsMarked int      `json:"corrections_marked"`
	Warnings          []string `json:"warnings,omitempty"`
	Errors            []string `json:"errors,omitempty"`
}

type Result struct {
	GeneratedAt          time.Time          `json:"generated_at"`
	LocationLabel        string             `json:"location_label,omitempty"`
	Complete             bool               `json:"complete"`
	Warnings             []string           `json:"warnings,omitempty"`
	Errors               []string           `json:"errors,omitempty"`
	Coverage             []MerchantCoverage `json:"coverage"`
	Candidates           []Candidate        `json:"candidates"`
	QualifyingOffers     []Candidate        `json:"qualifying_offers"`
	NewQualifyingOffers  []Candidate        `json:"new_qualifying_offers"`
	NewReviewCandidates  []Candidate        `json:"new_review_candidates"`
}
