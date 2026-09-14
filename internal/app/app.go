package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zcjb-ca/giftcard-watch/internal/config"
	"github.com/zcjb-ca/giftcard-watch/internal/extract"
	"github.com/zcjb-ca/giftcard-watch/internal/flipp"
	"github.com/zcjb-ca/giftcard-watch/internal/model"
	"github.com/zcjb-ca/giftcard-watch/internal/scan"
)

type Options struct {
	RawDirectory      string
	OCRCacheDirectory string
	Now               func() time.Time
}

func Run(ctx context.Context, cfg config.Config, options Options) model.Result {
	now := time.Now()
	if options.Now != nil {
		now = options.Now()
	}
	result := model.Result{
		GeneratedAt:   now.UTC(),
		LocationLabel: cfg.LocationLabel,
		Complete:      true,
		Coverage:      make([]model.MerchantCoverage, len(cfg.Merchants)),
	}
	coverageIndex := make(map[string]int, len(cfg.Merchants))
	for index, merchant := range cfg.Merchants {
		result.Coverage[index] = model.MerchantCoverage{
			Merchant: merchant.Name,
			Required: merchant.Required,
		}
		coverageIndex[merchant.Name] = index
	}

	client := flipp.New(
		cfg.BaseURL,
		cfg.AssetBaseURL,
		cfg.Locale,
		cfg.PostalCode,
		time.Duration(cfg.HTTPTimeoutSeconds)*time.Second,
	)
	analyzer := extract.New(cfg)

	flyers, rawList, err := client.ListFlyers(ctx)
	if err != nil {
		result.Complete = false
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	if options.RawDirectory != "" {
		if err := writeRaw(options.RawDirectory, "flyers.json", rawList); err != nil {
			result.Warnings = append(result.Warnings, "could not save raw flyer list: "+err.Error())
		}
	}

	selected := make([]model.Flyer, 0)
	for _, flyer := range flyers {
		merchant, matched := cfg.MatchMerchant(flyer.Merchant)
		if !matched || !flyer.IsCurrent(now) {
			continue
		}
		flyer.CanonicalMerchant = merchant.Name
		selected = append(selected, flyer)
		result.Coverage[coverageIndex[merchant.Name]].FlyersFound++
	}

	for index := range result.Coverage {
		coverage := &result.Coverage[index]
		if coverage.FlyersFound == 0 {
			message := "no current matching flyer was returned"
			if coverage.Required {
				coverage.Errors = append(coverage.Errors, message)
				result.Complete = false
			} else {
				coverage.Warnings = append(coverage.Warnings, message)
			}
		}
	}
	if len(selected) == 0 {
		result.Complete = false
		result.Errors = append(result.Errors, "no current target flyers were found")
		return result
	}

	var ocrEngine *scan.OCR
	ocrReady := false
	if cfg.OCR.Enabled {
		ocrEngine = scan.NewOCR(
			cfg.OCR.Command,
			cfg.OCR.Language,
			time.Duration(cfg.OCR.PageTimeoutSeconds)*time.Second,
		)
		if err := ocrEngine.Available(); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
			if cfg.OCR.Required {
				result.Complete = false
				result.Errors = append(result.Errors, "required OCR engine is unavailable")
			}
		} else {
			ocrReady = true
		}
	} else if cfg.OCR.Required {
		result.Complete = false
		result.Errors = append(result.Errors, "OCR is required but disabled")
	}

	candidateByID := make(map[string]model.Candidate)
	for _, flyer := range selected {
		index := coverageIndex[flyer.CanonicalMerchant]
		coverage := &result.Coverage[index]

		detail, rawDetail, err := client.FetchDetail(ctx, flyer)
		if err != nil {
			coverage.Errors = append(coverage.Errors, fmt.Sprintf("flyer %s detail: %v", flyer.ID, err))
			result.Complete = false
			continue
		}
		coverage.DetailsFetched++
		if options.RawDirectory != "" {
			name := safeName(flyer.CanonicalMerchant + "-" + flyer.ID + ".json")
			if err := writeRaw(options.RawDirectory, name, rawDetail); err != nil {
				coverage.Warnings = append(coverage.Warnings, "could not save raw detail: "+err.Error())
			}
		}

		if detail.HasCorrections {
			coverage.CorrectionsMarked++
			coverage.Warnings = append(coverage.Warnings, fmt.Sprintf("flyer %s is marked as corrected", flyer.ID))
		}
		coverage.ItemsScanned += len(detail.Items)
		if len(detail.Items) == 0 {
			coverage.Errors = append(coverage.Errors, fmt.Sprintf("flyer %s returned no structured items", flyer.ID))
			result.Complete = false
		}

		emptyItems := 0
		for _, item := range detail.Items {
			text := scan.FlattenStrings(item)
			if text == "" {
				emptyItems++
				continue
			}
			source := model.Source{
				Kind:      "structured_json",
				Merchant:  flyer.CanonicalMerchant,
				FlyerID:   flyer.ID,
				ValidFrom: dateString(flyer.ValidFrom),
				ValidTo:   dateString(flyer.ValidTo),
			}
			if candidate, ok := analyzer.Analyze(text, source); ok {
				candidateByID[candidate.ID] = candidate
			}
		}
		if emptyItems > 0 {
			coverage.Warnings = append(coverage.Warnings,
				fmt.Sprintf("flyer %s contained %d items without searchable text", flyer.ID, emptyItems))
		}

		coverage.PagesDeclared += detail.DeclaredPages
		coverage.PageImagesFound += detail.DeclaredPages - detail.MissingPageImages
		if cfg.OCR.Required && detail.DeclaredPages == 0 {
			coverage.Errors = append(coverage.Errors, fmt.Sprintf("flyer %s exposed no page geometry for OCR", flyer.ID))
			result.Complete = false
		}
		if cfg.OCR.Required && detail.MissingPageImages > 0 {
			coverage.Errors = append(coverage.Errors,
				fmt.Sprintf("flyer %s has %d page records without a usable raster source", flyer.ID, detail.MissingPageImages))
			result.Complete = false
		}

		if !ocrReady {
			continue
		}
		fingerprint := detailFingerprint(rawDetail)
		for _, page := range detail.Pages {
			if !page.HasRasterSource() {
				continue
			}

			text, cached, err := cachedOCR(options.OCRCacheDirectory, flyer.ID, page.Number, fingerprint)
			if err != nil {
				coverage.Warnings = append(coverage.Warnings,
					fmt.Sprintf("flyer %s page %d OCR cache read: %v", flyer.ID, page.Number, err))
			}
			if cached {
				coverage.PagesOCRed++
				coverage.PagesFromCache++
			} else {
				text, err = ocrEngine.ScanPage(ctx, page)
				if err != nil {
					coverage.Errors = append(coverage.Errors,
						fmt.Sprintf("flyer %s page %d OCR: %v", flyer.ID, page.Number, err))
					result.Complete = false
					continue
				}
				coverage.PagesOCRed++
				if err := saveOCR(options.OCRCacheDirectory, flyer.ID, page.Number, fingerprint, text); err != nil {
					coverage.Warnings = append(coverage.Warnings,
						fmt.Sprintf("flyer %s page %d OCR cache write: %v", flyer.ID, page.Number, err))
				}
			}
			if strings.TrimSpace(text) == "" {
				coverage.Errors = append(coverage.Errors,
					fmt.Sprintf("flyer %s page %d OCR returned no text", flyer.ID, page.Number))
				result.Complete = false
				continue
			}
			source := model.Source{
				Kind:      "ocr",
				Merchant:  flyer.CanonicalMerchant,
				FlyerID:   flyer.ID,
				Page:      page.Number,
				ImageURL:  page.ImageURL,
				ValidFrom: dateString(flyer.ValidFrom),
				ValidTo:   dateString(flyer.ValidTo),
			}
			if candidate, ok := analyzer.Analyze(text, source); ok {
				candidateByID[candidate.ID] = candidate
			}
		}
	}

	result.Candidates = make([]model.Candidate, 0, len(candidateByID))
	for _, candidate := range candidateByID {
		result.Candidates = append(result.Candidates, candidate)
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		left, right := result.Candidates[i], result.Candidates[j]
		if left.Source.Merchant != right.Source.Merchant {
			return left.Source.Merchant < right.Source.Merchant
		}
		if left.Reward.ReturnPercent != right.Reward.ReturnPercent {
			return left.Reward.ReturnPercent > right.Reward.ReturnPercent
		}
		return left.ID < right.ID
	})
	return result
}

func cachedOCR(directory, flyerID string, page int, fingerprint string) (string, bool, error) {
	if directory == "" {
		return "", false, nil
	}
	body, err := os.ReadFile(ocrCachePath(directory, flyerID, page, fingerprint))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(body), true, nil
}

func saveOCR(directory, flyerID string, page int, fingerprint, text string) error {
	if directory == "" {
		return nil
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	path := ocrCachePath(directory, flyerID, page, fingerprint)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(text), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func ocrCachePath(directory, flyerID string, page int, fingerprint string) string {
	name := safeName(fmt.Sprintf("%s-page-%d-%s.txt", flyerID, page, fingerprint))
	return filepath.Join(directory, name)
}

func detailFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func writeRaw(directory, name string, body []byte) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	var pretty any
	if json.Unmarshal(body, &pretty) == nil {
		if formatted, err := json.MarshalIndent(pretty, "", "  "); err == nil {
			body = append(formatted, '\n')
		}
	}
	return os.WriteFile(filepath.Join(directory, name), body, 0o600)
}

func safeName(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

func dateString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
