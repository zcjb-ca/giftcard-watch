package flipp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

const maxResponseBytes = 64 << 20

type Client struct {
	baseURL    string
	locale     string
	postalCode string
	http       *http.Client
}

type Detail struct {
	Items             []map[string]any
	Pages             []model.Page
	DeclaredPages     int
	MissingPageImages int
	HasCorrections    bool
}

func New(baseURL, locale, postalCode string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		locale:     locale,
		postalCode: postalCode,
		http:       &http.Client{Timeout: timeout},
	}
}

func (c *Client) ListFlyers(ctx context.Context) ([]model.Flyer, []byte, error) {
	root, raw, err := c.getJSON(ctx, "/flyers", "flyer list")
	if err != nil {
		return nil, nil, err
	}
	maps := flyerMaps(root)
	flyers := make([]model.Flyer, 0, len(maps))
	for _, value := range maps {
		id := firstValue(value, "id", "flyer_id")
		merchant := firstValue(value, "merchant_name", "merchant", "name")
		if id == "" || merchant == "" {
			continue
		}
		flyers = append(flyers, model.Flyer{
			ID:        id,
			Merchant:  merchant,
			ValidFrom: firstTime(value, "valid_from", "start_date", "available_from"),
			ValidTo:   firstTime(value, "valid_to", "end_date", "available_to"),
		})
	}
	return flyers, raw, nil
}

func (c *Client) FetchDetail(ctx context.Context, flyerID string) (Detail, []byte, error) {
	if flyerID == "" {
		return Detail{}, nil, errors.New("flyer ID is empty")
	}
	root, raw, err := c.getJSON(ctx, "/flyers/"+url.PathEscape(flyerID), "flyer detail")
	if err != nil {
		return Detail{}, nil, err
	}
	object, ok := root.(map[string]any)
	if !ok {
		return Detail{}, raw, errors.New("flyer detail root is not an object")
	}
	items := itemMaps(object)
	pages, declared, missing := pageList(object["pages"])
	return Detail{
		Items:             items,
		Pages:             pages,
		DeclaredPages:     declared,
		MissingPageImages: missing,
		HasCorrections:    boolValue(object["has_corrections"]),
	}, raw, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint, label string) (any, []byte, error) {
	requestURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: invalid base URL: %w", label, err)
	}
	requestURL.Path = path.Join(requestURL.Path, endpoint)
	query := requestURL.Query()
	query.Set("locale", c.locale)
	query.Set("postal_code", c.postalCode)
	requestURL.RawQuery = query.Encode()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * 400 * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, nil, ctx.Err()
			case <-timer.C:
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: create request: %w", label, err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "giftcard-watch/0.1 (+https://github.com/zcjb-ca/giftcard-watch)")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s request failed: %w", label, err)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("%s response read failed: %w", label, readErr)
			continue
		}
		if len(body) > maxResponseBytes {
			return nil, nil, fmt.Errorf("%s response exceeded %d bytes", label, maxResponseBytes)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("%s returned HTTP %d", label, resp.StatusCode)
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				continue
			}
			return nil, nil, lastErr
		}

		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var root any
		if err := decoder.Decode(&root); err != nil {
			return nil, body, fmt.Errorf("%s returned invalid JSON: %w", label, err)
		}
		return root, body, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s failed after retries", label)
	}
	return nil, nil, lastErr
}

func flyerMaps(root any) []map[string]any {
	if values, ok := root.([]any); ok {
		return mapsFromSlice(values)
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"flyers", "publications", "results"} {
		if values, ok := object[key].([]any); ok {
			return mapsFromSlice(values)
		}
	}
	return nil
}

func itemMaps(root map[string]any) []map[string]any {
	for _, key := range []string{"items", "flyer_items", "ecom_items"} {
		if values, ok := root[key].([]any); ok {
			return mapsFromSlice(values)
		}
	}
	keys := make([]string, 0, len(root))
	for key := range root {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values, ok := root[key].([]any)
		if !ok {
			continue
		}
		maps := mapsFromSlice(values)
		for _, value := range maps {
			if firstValue(value, "name", "short_name", "description") != "" {
				return maps
			}
		}
	}
	return nil
}

func mapsFromSlice(values []any) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func pageList(value any) ([]model.Page, int, int) {
	values, ok := value.([]any)
	if !ok {
		return nil, 0, 0
	}
	pages := make([]model.Page, 0, len(values))
	missing := 0
	for index, value := range values {
		page := model.Page{Number: index + 1}
		switch typed := value.(type) {
		case string:
			page.ImageURL = normalizeImageURL(typed)
		case map[string]any:
			if n := firstInteger(typed, "page_number", "page", "number", "index"); n > 0 {
				page.Number = n
			}
			page.ImageURL = bestImageURL(typed)
		}
		if page.ImageURL == "" {
			missing++
		}
		pages = append(pages, page)
	}
	return pages, len(values), missing
}

type imageChoice struct {
	url   string
	score int
}

func bestImageURL(root map[string]any) string {
	choices := make([]imageChoice, 0)
	var visit func(any, string)
	visit = func(value any, keyPath string) {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				visit(typed[key], keyPath+"."+strings.ToLower(key))
			}
		case []any:
			for _, child := range typed {
				visit(child, keyPath)
			}
		case string:
			candidate := normalizeImageURL(typed)
			if candidate == "" {
				return
			}
			score := 0
			for token, points := range map[string]int{
				"image": 6, "scan": 6, "zoom": 5, "large": 4,
				"high": 4, "page": 3, "url": 1, "thumb": -5,
				"cutout": -6, "logo": -6,
			} {
				if strings.Contains(keyPath, token) {
					score += points
				}
			}
			lowerURL := strings.ToLower(candidate)
			if strings.Contains(lowerURL, ".jpg") || strings.Contains(lowerURL, ".jpeg") ||
				strings.Contains(lowerURL, ".png") || strings.Contains(lowerURL, ".webp") {
				score += 2
			}
			if score > 0 {
				choices = append(choices, imageChoice{url: candidate, score: score})
			}
		}
	}
	visit(root, "")
	sort.SliceStable(choices, func(i, j int) bool { return choices[i].score > choices[j].score })
	if len(choices) == 0 {
		return ""
	}
	return choices[0].url
}

func normalizeImageURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") {
		value = "https://" + strings.TrimPrefix(value, "http://")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return value
}

func firstValue(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return strings.TrimSpace(typed)
				}
			case json.Number:
				return typed.String()
			case float64:
				return strconv.FormatFloat(typed, 'f', -1, 64)
			case int:
				return strconv.Itoa(typed)
			}
		}
	}
	return ""
}

func firstInteger(object map[string]any, keys ...string) int {
	value := firstValue(object, keys...)
	n, _ := strconv.Atoi(value)
	return n
}

func firstTime(object map[string]any, keys ...string) time.Time {
	value := firstValue(object, keys...)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}
