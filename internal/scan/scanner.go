package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	_ "image/jpeg"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/zcjb-ca/giftcard-watch/internal/model"
)

const (
	maxPageBytes = 32 << 20
	tileSize     = 256
)

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
	if !page.HasRasterSource() {
		return "", errors.New("page has no usable raster source")
	}
	pageCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	name, err := o.materializePage(pageCtx, page)
	if err != nil {
		return "", err
	}
	defer os.Remove(name)

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

func (o *OCR) materializePage(ctx context.Context, page model.Page) (string, error) {
	file, err := os.CreateTemp("", "giftcard-watch-page-*.png")
	if err != nil {
		return "", fmt.Errorf("create temporary page image: %w", err)
	}
	name := file.Name()
	cleanup := func(err error) (string, error) {
		file.Close()
		os.Remove(name)
		return "", err
	}

	if page.ImageURL != "" {
		body, err := o.downloadBytes(ctx, page.ImageURL)
		if err != nil {
			return cleanup(err)
		}
		if _, err := file.Write(body); err != nil {
			return cleanup(fmt.Errorf("write temporary page image: %w", err))
		}
		if err := file.Close(); err != nil {
			os.Remove(name)
			return "", fmt.Errorf("close temporary page image: %w", err)
		}
		return name, nil
	}

	rendered, err := o.renderTiles(ctx, page)
	if err != nil {
		return cleanup(err)
	}
	if err := png.Encode(file, rendered); err != nil {
		return cleanup(fmt.Errorf("encode stitched flyer page: %w", err))
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("close stitched flyer page: %w", err)
	}
	return name, nil
}

func (o *OCR) renderTiles(ctx context.Context, page model.Page) (image.Image, error) {
	if page.Resolution <= 0 || page.Right <= page.Left || page.Top <= page.Bottom {
		return nil, errors.New("invalid flyer tile geometry")
	}
	scale := page.Resolution
	tileWorldSize := float64(tileSize) * scale
	width := int(math.Ceil(float64(page.Right-page.Left) / scale))
	height := int(math.Ceil(float64(page.Top-page.Bottom) / scale))
	if width <= 0 || height <= 0 || width*height > 30_000_000 {
		return nil, fmt.Errorf("unsafe stitched page dimensions %dx%d", width, height)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	minX := int(math.Floor(float64(page.Left) / tileWorldSize))
	maxX := int(math.Ceil(float64(page.Right)/tileWorldSize)) - 1
	minY := int(math.Floor(float64(page.Bottom-page.CanvasBottom) / tileWorldSize))
	maxY := int(math.Ceil(float64(page.Top-page.CanvasBottom)/tileWorldSize)) - 1
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}

	for tileY := minY; tileY <= maxY; tileY++ {
		for tileX := minX; tileX <= maxX; tileX++ {
			tileURL := fmt.Sprintf("%s%d_%d_%d.jpg",
				page.TileBaseURL, page.ResolutionIndex, tileX, tileY)
			tile, err := o.downloadImage(ctx, tileURL)
			if err != nil {
				return nil, fmt.Errorf("download flyer tile %d,%d: %w", tileX, tileY, err)
			}
			tileLeft := float64(tileX) * tileWorldSize
			tileTop := float64(page.CanvasBottom) + float64(tileY+1)*tileWorldSize
			destinationX := int(math.Round((tileLeft - float64(page.Left)) / scale))
			destinationY := int(math.Round((float64(page.Top) - tileTop) / scale))
			bounds := tile.Bounds()
			destination := image.Rect(
				destinationX,
				destinationY,
				destinationX+bounds.Dx(),
				destinationY+bounds.Dy(),
			)
			draw.Draw(canvas, destination, tile, bounds.Min, draw.Src)
		}
	}
	return canvas, nil
}

func (o *OCR) downloadImage(ctx context.Context, imageURL string) (image.Image, error) {
	body, err := o.downloadBytes(ctx, imageURL)
	if err != nil {
		return nil, err
	}
	decoded, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return decoded, nil
}

func (o *OCR) downloadBytes(ctx context.Context, imageURL string) ([]byte, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("image URL is not a valid HTTPS URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create image request: %w", err)
	}
	req.Header.Set("User-Agent", "giftcard-watch/0.1 (+https://github.com/zcjb-ca/giftcard-watch)")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download image returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if len(body) > maxPageBytes {
		return nil, fmt.Errorf("image exceeded %d bytes", maxPageBytes)
	}
	return body, nil
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
