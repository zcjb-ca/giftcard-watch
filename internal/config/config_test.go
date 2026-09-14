package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesPostalCodeEnvironment(t *testing.T) {
	t.Setenv("GIFTWATCH_POSTAL_CODE", "k1p 1j1")
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	body := []byte(`{
  "location_label": "Ottawa, ON",
  "ocr": {
    "enabled": false,
    "required": false,
    "command": "",
    "language": "",
    "page_timeout_seconds": 0
  }
}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.PostalCode != "K1P1J1" {
		t.Fatalf("PostalCode = %q, want K1P1J1", cfg.PostalCode)
	}
	if cfg.LocationLabel != "Ottawa, ON" {
		t.Fatalf("LocationLabel = %q", cfg.LocationLabel)
	}
}

func TestMatchMerchantAcceptsKnownAlias(t *testing.T) {
	cfg := Default()
	got, ok := cfg.MatchMerchant("Real Canadian Superstore - Ontario")
	if !ok {
		t.Fatal("MatchMerchant() did not match")
	}
	if got.Name != "Real Canadian Superstore" {
		t.Fatalf("merchant = %q", got.Name)
	}
}
