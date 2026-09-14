package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Merchant struct {
	Name     string   `json:"name"`
	Aliases  []string `json:"aliases"`
	Required bool     `json:"required"`
}

type Brand struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

type PointProgram struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	CADPerPoint float64  `json:"cad_per_point"`
}

type OCR struct {
	Enabled            bool   `json:"enabled"`
	Required           bool   `json:"required"`
	Command            string `json:"command"`
	Language           string `json:"language"`
	PageTimeoutSeconds int    `json:"page_timeout_seconds"`
}

type Config struct {
	BaseURL                 string         `json:"base_url"`
	AssetBaseURL            string         `json:"asset_base_url"`
	Locale                  string         `json:"locale"`
	PostalCode              string         `json:"postal_code"`
	LocationLabel           string         `json:"location_label"`
	MinimumReturnPercent    float64        `json:"minimum_return_percent"`
	HTTPTimeoutSeconds      int            `json:"http_timeout_seconds"`
	Merchants               []Merchant     `json:"merchants"`
	Brands                  []Brand        `json:"brands"`
	PointPrograms           []PointProgram `json:"point_programs"`
	OCR                     OCR            `json:"ocr"`
}

var canadianPostalCode = regexp.MustCompile(`^[A-Z]\d[A-Z]\d[A-Z]\d$`)

func Default() Config {
	return Config{
		BaseURL:              "https://backflipp.wishabi.com/flipp",
		AssetBaseURL:         "https://dam.flippenterprise.net/api/flipp",
		Locale:               "en-ca",
		MinimumReturnPercent: 8,
		HTTPTimeoutSeconds:   25,
		Merchants: []Merchant{
			{Name: "Loblaws", Aliases: []string{"Loblaws"}, Required: true},
			{Name: "Shoppers Drug Mart", Aliases: []string{"Shoppers Drug Mart", "Shoppers"}, Required: true},
			{Name: "Real Canadian Superstore", Aliases: []string{"Real Canadian Superstore", "RCSS", "Superstore"}, Required: true},
			{Name: "No Frills", Aliases: []string{"No Frills", "nofrills"}, Required: true},
			{Name: "Sobeys", Aliases: []string{"Sobeys"}, Required: true},
			{Name: "FreshCo", Aliases: []string{"FreshCo", "Fresh Co"}, Required: true},
			{Name: "Metro", Aliases: []string{"Metro"}, Required: true},
			{Name: "Canadian Tire", Aliases: []string{"Canadian Tire"}, Required: true},
			{Name: "Costco", Aliases: []string{"Costco"}, Required: false},
		},
		Brands: []Brand{
			{Name: "Best Buy", Aliases: []string{"Best Buy", "BestBuy"}},
			{Name: "Apple", Aliases: []string{"Apple"}},
			{Name: "Esso", Aliases: []string{"Esso"}},
			{Name: "Shell", Aliases: []string{"Shell"}},
			{Name: "Amazon", Aliases: []string{"Amazon"}},
			{Name: "Uber", Aliases: []string{"Uber", "Uber Eats"}},
			{Name: "DoorDash", Aliases: []string{"DoorDash"}},
			{Name: "Air Canada", Aliases: []string{"Air Canada"}},
			{Name: "Porter", Aliases: []string{"Porter"}},
			{Name: "Indigo", Aliases: []string{"Indigo"}},
			{Name: "Google Play", Aliases: []string{"Google Play"}},
			{Name: "PlayStation", Aliases: []string{"PlayStation", "PSN"}},
			{Name: "Nintendo", Aliases: []string{"Nintendo"}},
			{Name: "Moxies", Aliases: []string{"Moxies", "Moxie's"}},
		},
		PointPrograms: []PointProgram{
			{Name: "PC Optimum", Aliases: []string{"PC Optimum", "Optimum points"}, CADPerPoint: 0.001},
			{Name: "Scene+", Aliases: []string{"Scene+", "Scene Plus"}, CADPerPoint: 0.01},
		},
		OCR: OCR{
			Enabled:            true,
			Required:           true,
			Command:            "tesseract",
			Language:           "eng",
			PageTimeoutSeconds: 45,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return Config{}, fmt.Errorf("open config: %w", err)
		}
		defer f.Close()
		decoder := json.NewDecoder(f)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("decode config: %w", err)
		}
	}

	if v := firstEnv("GIFTWATCH_POSTAL_CODE", "POSTAL_CODE"); v != "" {
		cfg.PostalCode = v
	}
	if v := os.Getenv("GIFTWATCH_MIN_RETURN_PERCENT"); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse GIFTWATCH_MIN_RETURN_PERCENT: %w", err)
		}
		cfg.MinimumReturnPercent = n
	}

	cfg.PostalCode = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(cfg.PostalCode), " ", ""))
	cfg.Locale = strings.ToLower(strings.TrimSpace(cfg.Locale))
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.AssetBaseURL = strings.TrimRight(strings.TrimSpace(cfg.AssetBaseURL), "/")
	cfg.LocationLabel = strings.TrimSpace(cfg.LocationLabel)
	cfg.OCR.Command = strings.TrimSpace(cfg.OCR.Command)
	cfg.OCR.Language = strings.TrimSpace(cfg.OCR.Language)

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	switch {
	case c.BaseURL == "":
		return errors.New("base_url is required")
	case c.AssetBaseURL == "":
		return errors.New("asset_base_url is required")
	case c.Locale == "":
		return errors.New("locale is required")
	case c.PostalCode == "":
		return errors.New("postal code is required; set GIFTWATCH_POSTAL_CODE or POSTAL_CODE")
	case strings.HasSuffix(c.Locale, "-ca") && !canadianPostalCode.MatchString(c.PostalCode):
		return errors.New("Canadian postal code must contain six characters in A1A1A1 form")
	case c.MinimumReturnPercent < 0:
		return errors.New("minimum_return_percent cannot be negative")
	case c.HTTPTimeoutSeconds <= 0:
		return errors.New("http_timeout_seconds must be positive")
	case len(c.Merchants) == 0:
		return errors.New("at least one merchant is required")
	case len(c.Brands) == 0:
		return errors.New("at least one brand is required")
	case c.OCR.Enabled && c.OCR.Command == "":
		return errors.New("ocr.command is required when OCR is enabled")
	case c.OCR.Enabled && c.OCR.PageTimeoutSeconds <= 0:
		return errors.New("ocr.page_timeout_seconds must be positive")
	}
	for _, p := range c.PointPrograms {
		if p.Name == "" || len(p.Aliases) == 0 || p.CADPerPoint <= 0 {
			return fmt.Errorf("invalid point program %q", p.Name)
		}
	}
	return nil
}

func (c Config) MatchMerchant(raw string) (Merchant, bool) {
	value := normalized(raw)
	for _, merchant := range c.Merchants {
		for _, alias := range append([]string{merchant.Name}, merchant.Aliases...) {
			needle := normalized(alias)
			if needle != "" && strings.Contains(value, needle) {
				return merchant, true
			}
		}
	}
	return Merchant{}, false
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func normalized(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
