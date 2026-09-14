package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

func Write(result model.Result, jsonPath, markdownPath string) error {
	if jsonPath != "" {
		body, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("encode JSON report: %w", err)
		}
		if err := atomicWrite(jsonPath, append(body, '\n')); err != nil {
			return fmt.Errorf("write JSON report: %w", err)
		}
	}
	if markdownPath != "" {
		if err := atomicWrite(markdownPath, []byte(Markdown(result))); err != nil {
			return fmt.Errorf("write Markdown report: %w", err)
		}
	}
	return nil
}

func Markdown(result model.Result) string {
	var b strings.Builder
	b.WriteString("# Gift-card watch report\n\n")
	fmt.Fprintf(&b, "- Generated: %s\n", result.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))
	if result.LocationLabel != "" {
		fmt.Fprintf(&b, "- Region: %s\n", escape(result.LocationLabel))
	}
	if result.Complete {
		b.WriteString("- Coverage: **complete**\n")
	} else {
		b.WriteString("- Coverage: **INCOMPLETE — do not interpret this as no offers**\n")
	}
	fmt.Fprintf(&b, "- New qualifying offers: **%d**\n", len(result.NewQualifyingOffers))
	fmt.Fprintf(&b, "- New candidates needing review: **%d**\n\n", len(result.NewReviewCandidates))

	if len(result.Errors) > 0 {
		b.WriteString("## Scan errors\n\n")
		for _, value := range result.Errors {
			fmt.Fprintf(&b, "- %s\n", escape(value))
		}
		b.WriteString("\n")
	}
	if len(result.Warnings) > 0 {
		b.WriteString("## Scan warnings\n\n")
		for _, value := range result.Warnings {
			fmt.Fprintf(&b, "- %s\n", escape(value))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Coverage\n\n")
	b.WriteString("| Merchant | Flyers | Details | Items | Pages | OCR | Status |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---|\n")
	for _, coverage := range result.Coverage {
		status := "ok"
		if len(coverage.Errors) > 0 {
			status = "error: " + strings.Join(coverage.Errors, "; ")
		} else if len(coverage.Warnings) > 0 {
			status = "warning: " + strings.Join(coverage.Warnings, "; ")
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d/%d | %d | %s |\n",
			escape(coverage.Merchant),
			coverage.FlyersFound,
			coverage.DetailsFetched,
			coverage.ItemsScanned,
			coverage.PageImagesFound,
			coverage.PagesDeclared,
			coverage.PagesOCRed,
			escape(status),
		)
	}
	b.WriteString("\n")

	writeCandidates(&b, "New qualifying offers", result.NewQualifyingOffers)
	writeCandidates(&b, "All current qualifying offers", result.QualifyingOffers)
	writeCandidates(&b, "New candidates needing review", result.NewReviewCandidates)
	return b.String()
}

func writeCandidates(b *strings.Builder, title string, candidates []model.Candidate) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(candidates) == 0 {
		b.WriteString("None.\n\n")
		return
	}
	b.WriteString("| Merchant | Brands | Return | Reward | Source | Evidence |\n")
	b.WriteString("|---|---|---:|---|---|---|\n")
	for _, candidate := range candidates {
		source := candidate.Source.Kind
		if candidate.Source.Page > 0 {
			source += fmt.Sprintf(" page %d", candidate.Source.Page)
		}
		if candidate.Source.ImageURL != "" {
			source = fmt.Sprintf("[%s](%s)", escape(source), candidate.Source.ImageURL)
		}
		reward := candidate.Reward.Kind
		if candidate.Reward.Program != "" {
			reward += " / " + candidate.Reward.Program
		}
		fmt.Fprintf(b, "| %s | %s | %.2f%% | %s | %s | %s |\n",
			escape(candidate.Source.Merchant),
			escape(strings.Join(candidate.Brands, ", ")),
			candidate.Reward.ReturnPercent,
			escape(reward),
			source,
			escape(short(candidate.RawText, 240)),
		)
	}
	b.WriteString("\n")
}

func atomicWrite(path string, body []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func short(value string, limit int) string {
	runes := []rune(strings.Join(strings.Fields(value), " "))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func escape(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
