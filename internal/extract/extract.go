package extract

import (
	"crypto/sha256"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/zcjb-ca/giftcard-watch/internal/config"
	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

var (
	giftSignal = regexp.MustCompile(`(?i)\b(?:e-?\s*)?gift\s*cards?\b|\bprepaid\s+cards?\b`)
	rewardSignal = regexp.MustCompile(`(?i)\bpoints?\b|\b(?:save|get|earn|collect|bonus|off|back)\b|ct\s+money|scene\+?|optimum`)
	percentPattern = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*%\s*(?:off|back)`)
	pointsPattern = regexp.MustCompile(`(?i)([0-9][0-9,]*)\s*(?:bonus\s+)?(?:pc\s+optimum\s+|scene\+?\s+)?points?`)
	spendPattern = regexp.MustCompile(`(?i)(?:for\s+every|when\s+you\s+(?:buy|spend)|(?:buy|spend))\s+(?:at\s+least\s+)?\$?\s*([0-9]+(?:\.[0-9]{1,2})?)`)
	ctMoneyPattern = regexp.MustCompile(`(?i)\$\s*([0-9]+(?:\.[0-9]{1,2})?)\s*(?:in\s+)?(?:ct|canadian\s+tire)\s+money`)
	faceCostPattern = regexp.MustCompile(`(?i)\$?\s*([0-9]+(?:\.[0-9]{1,2})?)\s+[a-z0-9&+' -]{0,50}(?:e-?\s*gift|gift)\s*cards?.{0,100}?(?:for|only|pay)\s+\$?\s*([0-9]+(?:\.[0-9]{1,2})?)`)
)

type Analyzer struct {
	cfg config.Config
}

func New(cfg config.Config) *Analyzer {
	return &Analyzer{cfg: cfg}
}

func (a *Analyzer) Analyze(text string, source model.Source) (model.Candidate, bool) {
	text = clean(text)
	if text == "" {
		return model.Candidate{}, false
	}
	brands := a.matchBrands(text)
	if !giftSignal.MatchString(text) && !(len(brands) > 0 && rewardSignal.MatchString(text)) {
		return model.Candidate{}, false
	}

	reward, parsed := a.parseReward(text)
	candidate := model.Candidate{
		Brands:     brands,
		RawText:    truncate(text, 1600),
		Source:     source,
		Reward:     reward,
		Confidence: "high",
	}
	if source.Kind == "ocr" {
		candidate.Confidence = "medium"
	}
	switch {
	case !parsed:
		candidate.NeedsReview = true
		candidate.Reason = "gift-card signal found, but reward value could not be parsed"
	case len(brands) == 0:
		candidate.NeedsReview = true
		candidate.Reason = "promotion parsed, but participating gift-card brands were not identified"
	default:
		candidate.Qualifying = reward.ReturnPercent+0.000001 >= a.cfg.MinimumReturnPercent
	}
	candidate.ID = stableID(candidate)
	return candidate, true
}

func (a *Analyzer) parseReward(text string) (model.Reward, bool) {
	if match := percentPattern.FindStringSubmatch(text); len(match) == 2 {
		percent, ok := number(match[1])
		if ok && percent > 0 && percent <= 100 {
			return model.Reward{Kind: "direct_percent", ReturnPercent: percent}, true
		}
	}

	spend, hasSpend := firstNumber(spendPattern, text)
	if match := ctMoneyPattern.FindStringSubmatch(text); len(match) == 2 && hasSpend {
		rewardCAD, ok := number(match[1])
		if ok && rewardCAD > 0 && spend > 0 {
			return model.Reward{
				Kind:          "store_currency",
				Program:       "Canadian Tire Money",
				SpendCAD:      spend,
				RewardCAD:     rewardCAD,
				ReturnPercent: rewardCAD / spend * 100,
			}, true
		}
	}

	if match := pointsPattern.FindStringSubmatch(text); len(match) == 2 && hasSpend {
		points, ok := number(match[1])
		if ok && points > 0 && spend > 0 {
			if program, found := a.pointProgram(text); found {
				rewardCAD := points * program.CADPerPoint
				return model.Reward{
					Kind:          "points",
					Program:       program.Name,
					SpendCAD:      spend,
					Points:        points,
					RewardCAD:     rewardCAD,
					ReturnPercent: rewardCAD / spend * 100,
				}, true
			}
		}
	}

	if match := faceCostPattern.FindStringSubmatch(text); len(match) == 3 {
		face, faceOK := number(match[1])
		cost, costOK := number(match[2])
		if faceOK && costOK && face > 0 && cost > 0 && cost < face {
			return model.Reward{
				Kind:          "discounted_face_value",
				FaceValueCAD:  face,
				CostCAD:       cost,
				RewardCAD:     face - cost,
				ReturnPercent: (face - cost) / face * 100,
			}, true
		}
	}
	return model.Reward{}, false
}

func (a *Analyzer) matchBrands(text string) []string {
	lower := strings.ToLower(text)
	found := make([]string, 0)
	for _, brand := range a.cfg.Brands {
		for _, alias := range append([]string{brand.Name}, brand.Aliases...) {
			if containsAlias(lower, strings.ToLower(alias)) {
				found = append(found, brand.Name)
				break
			}
		}
	}
	sort.Strings(found)
	return found
}

func containsAlias(text, alias string) bool {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return false
	}
	for offset := 0; offset <= len(text)-len(alias); {
		index := strings.Index(text[offset:], alias)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(alias)
		beforeOK := start == 0 || !isASCIIAlphaNumeric(text[start-1])
		afterOK := end == len(text) || !isASCIIAlphaNumeric(text[end])
		if beforeOK && afterOK {
			return true
		}
		offset = start + 1
	}
	return false
}

func isASCIIAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func (a *Analyzer) pointProgram(text string) (config.PointProgram, bool) {
	lower := strings.ToLower(text)
	for _, program := range a.cfg.PointPrograms {
		for _, alias := range append([]string{program.Name}, program.Aliases...) {
			if strings.Contains(lower, strings.ToLower(alias)) {
				return program, true
			}
		}
	}
	return config.PointProgram{}, false
}

func firstNumber(pattern *regexp.Regexp, text string) (float64, bool) {
	match := pattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0, false
	}
	return number(match[1])
}

func number(value string) (float64, bool) {
	value = strings.ReplaceAll(value, ",", "")
	n, err := strconv.ParseFloat(value, 64)
	return n, err == nil
}

func stableID(candidate model.Candidate) string {
	key := strings.Join([]string{
		candidate.Source.Merchant,
		candidate.Source.FlyerID,
		fmt.Sprintf("%d", candidate.Source.Page),
		strings.Join(candidate.Brands, ","),
		candidate.Reward.Kind,
		fmt.Sprintf("%.4f", candidate.Reward.ReturnPercent),
		normalizeIDText(candidate.RawText),
	}, "|")
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum[:12])
}

func normalizeIDText(value string) string {
	value = strings.ToLower(value)
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func clean(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
