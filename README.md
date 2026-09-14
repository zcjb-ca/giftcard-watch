# giftcard-watch

A small Go program that checks Canadian weekly flyers for gift-card promotions, calculates their effective return, and refuses to report a clean "no offers" result when source coverage is incomplete.

The scanner deliberately does **not** use Flipp's item-search endpoint. It fetches the complete current flyer list for a postal code, retrieves every target flyer in full, scans all structured item fields, and OCRs every flyer page exposed by the detail response.

## What it watches

The default configuration targets:

- Loblaws
- Shoppers Drug Mart
- Real Canadian Superstore
- No Frills
- Sobeys
- FreshCo
- Metro
- Canadian Tire
- Costco when a Flipp flyer is available

Tracked gift-card brands include Best Buy, Apple, Esso, Shell, Amazon, Uber, DoorDash, Air Canada, Porter, Indigo, Google Play, PlayStation, Nintendo, and Moxies.

The default notification threshold is 8%.

## Accuracy behaviour

A run is marked incomplete when, for example:

- a required merchant has no matching current flyer;
- a flyer detail request fails;
- the detail has no structured items;
- page images are missing;
- required OCR is unavailable or fails.

An incomplete report prominently says not to interpret it as "no offers." Promotions that look relevant but cannot be valued or whose participating brands cannot be identified are retained as `needs_review` candidates.

Coverage applies to current content exposed by Flipp. Personalized app offers, authenticated member offers, and website-only promotions are outside the first version's scope.

## Requirements

- Go 1.23 or newer
- Tesseract OCR

On macOS:

~~~sh
brew install tesseract
~~~

## Local setup

Clone the repository and create a private local configuration:

~~~sh
git clone https://github.com/zcjb-ca/giftcard-watch.git
cd giftcard-watch
cp config/config.example.json config/config.json
export GIFTWATCH_POSTAL_CODE='A1A1A1'
~~~

`config/config.json`, generated reports, raw snapshots, and local state are ignored by Git.

Run the full scanner:

~~~sh
go run ./cmd/giftcardwatch \
  -config config/config.json \
  -output data/result.json \
  -report data/report.md
~~~

Run only the structured-data path while debugging:

~~~sh
go run ./cmd/giftcardwatch -no-ocr -raw-dir raw
~~~

Require a non-zero exit status for incomplete coverage:

~~~sh
go run ./cmd/giftcardwatch -strict
~~~

## Configuration

`config/config.example.json` contains the non-sensitive defaults. The postal code should normally be supplied through one of these environment variables:

1. `GIFTWATCH_POSTAL_CODE`
2. `POSTAL_CODE`

`GIFTWATCH_MIN_RETURN_PERCENT` optionally overrides the configured threshold.

The postal code is used only in requests. Reports contain `location_label`, not the postal code, so scheduled runs do not expose it in logs or artifacts.

## Outputs

- `data/result.json` — machine-readable coverage, candidates, and offers
- `data/report.md` — human-readable report and GitHub Actions summary
- `.giftcard-watch/state.json` — local deduplication state
- `raw/` — optional raw API snapshots when `-raw-dir` is supplied

State is retained for 120 days. A qualifying offer or review candidate is considered new only the first time its stable ID is observed.

## GitHub Actions

`ci.yml` runs unit tests and `go vet` on pull requests and pushes to `main`.

`scan.yml` runs daily and can also be started manually. Before using it:

1. Add an Actions secret named `POSTAL_CODE`.
2. Merge the implementation into `main`.
3. Optionally add a repository variable `ENABLE_ISSUES=true` to post new offers, review candidates, or incomplete scans to one rolling GitHub issue.

Issue notification is disabled by default because this repository is public. Reports are always available in the Actions summary and as a workflow artifact.

## Development

~~~sh
go test ./...
go vet ./...
~~~

The project intentionally uses only the Go standard library. Tesseract is invoked as an external process so OCR can run the same way on macOS and GitHub's Ubuntu runners.
