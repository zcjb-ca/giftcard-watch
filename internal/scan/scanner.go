package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

const maxPageBytes = 32 << 20

func FlattenStrings(value any) string {
	parts := make([]string, 0)
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				visit(typed[key])
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		case string:
			text := strings.TrimSpace(typed)
			if text != "" && !strings.HasPrefix(text, "http://") && !strings.HasPrefix(text, "https://") {
				parts = append(parts, text)
			}
		case json.Number:
			parts = append(parts, typed.String())
		case float64:
			parts = append(parts, fmt.Sprintf("%g", typed))
		case int:
			parts = append(parts, fmt.Sprintf("%d", typed))
		}
	}
	visit(value)
	return strings.Join(parts, " | ")
}

func ItemID(value map[string]any) string {
	for _, key := range []string{"id", "flyer_item_id", "item_id", "print_id"} {
		switch typed := value[key].(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case json.Number:
			return typed.String()
		case float64:
			return fmt.Sprintf("%.0f", typed)
		}
	}
	return ""
}

type OCR struct {
	Command  string
	Language string
	Timeout  time.Duration
	client   *http.Client
}

func NewOCR(command, language string, timeout time.Duration) *OCR {
	return &OCR{
		Command:  command,
		Language: language,
		Timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
	}
}

func (o *OCR) Available() error {
	if o.Command == "" {
		return errors.New("OCR command is empty")
	}
	_, err := exec.LookPath(o.Command)
	if err != nil {
		return fmt.Errorf("find OCR command %q: %w", o.Command, err)
	}
	return nil
}

func (o *OCR) ScanPage(ctx context.Context, page model.Page) (string, error) {
	parsed, err := url.Parse(page.ImageURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("page image URL is not a valid HTTPS URL")
	}
	pageCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(pageCtx, http.MethodGet, page.ImageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create page image request: %w", err)
	}
	req.Header.Set("User-Agent", "giftcard-watch/0.1 (+https://github.com/zcjb-ca/giftcard-watch)")
	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download page image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download page image returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return "", fmt.Errorf("read page image: %w", err)
	}
	if len(body) > maxPageBytes {
		return "", fmt.Errorf("page image exceeded %d bytes", maxPageBytes)
	}

	file, err := os.CreateTemp("", "giftcard-watch-page-*")
	if err != nil {
		return "", fmt.Errorf("create temporary page image: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(body); err != nil {
		file.Close()
		return "", fmt.Errorf("write temporary page image: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temporary page image: %w", err)
	}

	outputs := make([]string, 0, 2)
	for _, mode := range []string{"11", "6"} {
		command := exec.CommandContext(pageCtx, o.Command, name, "stdout", "-l", o.Language, "--psm", mode)
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			if pageCtx.Err() != nil {
				return "", fmt.Errorf("OCR timed out: %w", pageCtx.Err())
			}
			return "", fmt.Errorf("OCR mode %s failed: %w: %s", mode, err, strings.TrimSpace(stderr.String()))
		}
		outputs = append(outputs, stdout.String())
	}
	return mergeOCR(outputs...), nil
}

func mergeOCR(values ...string) string {
	seen := make(map[string]struct{})
	lines := make([]string, 0)
	for _, value := range values {
		for _, line := range strings.Split(value, "\n") {
			line = strings.Join(strings.Fields(line), " ")
			if line == "" {
				continue
			}
			key := strings.ToLower(line)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
