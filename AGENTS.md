# Datalastic Go SDK — Agent Guide

Reference for AI agents working with or on this SDK.

## Overview

Pure Go SDK for the Datalastic Maritime API. Zero external dependencies — stdlib only. Requires Go 1.21+.

Module: `github.com/datalastic/datalastic-go`

## Resources and Methods

All methods are on a `*Client` returned by `NewClient(apiKey, ...Option)`.

### Account
| Method | Description |
|---|---|
| `client.Stat()` | API key status and credit consumption. Free to call. |

### Vessels — `client.Vessels.*`
| Method | Params struct | Description |
|---|---|---|
| `Get` | `VesselParams` | Basic AIS position (uuid/mmsi/imo, one required) |
| `Pro` | `VesselParams` | Extended: ETA, ATD, draught, port names |
| `Bulk` | `VesselBulkParams` | Up to 100 vessels in one call |
| `InRadius` | `VesselInRadiusParams` | All vessels within a radius of a point/port/vessel |
| `History` | `VesselHistoryParams` | Historical positions since 2021-08-10 |
| `Info` | `VesselParams` | Static specs: dimensions, tonnage, year built |
| `Find` | `VesselFindParams` | Search by type, name, country, dimensions, year |
| `Estimated` | `VesselParams` | Satellite-estimated position (add-on, ext base URL) |

### Ports — `client.Ports.*`
| Method | Params struct | Description |
|---|---|---|
| `Find` | `PortFindParams` | Search by name, UNLOCODE, type, country, or lat/lon |
| `Get` | `PortGetParams` | Full detail including terminals |

### Routes — `client.Routes.*`
| Method | Params struct | Description |
|---|---|---|
| `Calculate` | `RouteParams` | Optimal sea route; returns GeoJSON + distance in nm. Free. |

### Maritime Intelligence — `client.Intel.*`
All Intel methods use the `maritime_reports` base URL and are add-ons.

| Method | Params struct | Description |
|---|---|---|
| `DryDock` | `IntelDryDockParams` | Dry-dock and survey dates |
| `Casualties` | `IntelDateRangeParams` | Casualty records |
| `Inspections` | `IntelDateRangeParams` | Port-state-control inspections |
| `SPD` | `IntelDateRangeParams` | Sales, purchases, demolitions |
| `Ownership` | `OwnershipParams` | Beneficial owner and management |
| `ClassSociety` | `ClassSocietyParams` | Classification society records |
| `Engine` | `EngineParams` | Engine and propulsion records |
| `Companies` | `CompanyParams` | Maritime company records |

### Reports — `client.Reports.*`
| Method | Params / arg | Description |
|---|---|---|
| `Submit` | `ReportSubmitParams` | Submit a generic async report job (POST, x-api-key header) |
| `InRadiusHistory` | `InRadiusHistoryParams` | Async historical area scan: all vessels in radius over a date range |
| `Get` | `reportID string` | Poll a report job by ID |
| `ListAll` | — | All report jobs for this key |

### Methods that also return metadata

Methods returning a plain slice have a `...WithMeta` sibling returning
`([]T, *Meta, error)`. The plain method delegates to the sibling, so both make
exactly one request.

| Plain method | Sibling |
|---|---|
| `Ports.Find` | `Ports.FindWithMeta` |
| `Intel.DryDock` | `Intel.DryDockWithMeta` |
| `Intel.Casualties` | `Intel.CasualtiesWithMeta` |
| `Intel.Inspections` | `Intel.InspectionsWithMeta` |
| `Intel.SPD` | `Intel.SPDWithMeta` |
| `Intel.Ownership` | `Intel.OwnershipWithMeta` |
| `Intel.ClassSociety` | `Intel.ClassSocietyWithMeta` |
| `Intel.Engine` | `Intel.EngineWithMeta` |
| `Intel.Companies` | `Intel.CompaniesWithMeta` |
| `Reports.ListAll` | `Reports.ListAllWithMeta` |

## Key Conventions

- **Auth is an HTTP header.** Both GET and POST requests set `x-api-key` on the
  request header. The key never appears in the URL query string or JSON body.
- **User-Agent.** Every request sends `datalastic-go/<Version>`. `Version` lives
  in `version.go` and is the single source of truth; the release git tag is
  exactly `"v" + Version`.
- **Three base URLs:** `BaseV0` (`/api/v0`) for core endpoints; `BaseExt`
  (`/api/ext`) for `Estimated` and `Calculate`; `BaseMR`
  (`/api/maritime_reports`) for all Intel methods.
- **Bulk uses repeated params:** `vessel_bulk` sends multiple `mmsi=` / `imo=` /
  `uuid=` values via `url.Values.Add`.
- **`VesselFindParams.VesselType` maps to the `type` query key** — not
  `vessel_type`.
- **`IncludeNullType` maps to the `_empty_` query param** on `Vessels.Find` and
  `Vessels.InRadius`. It is a `*bool`: nil omits the param, otherwise `true` or
  `false` is sent verbatim.
- **Find/search guards:** `Vessels.Find` requires at least one of `Name`,
  `VesselType`, `TypeSpecific`, `CountryISO`, `IncludeNullType`, or any range
  bound. `Fuzzy` and `Next` do not count. `Ports.Find` requires at least one of
  name, UUID, port type, country, UNLOCODE, or lat+lon; `radius` is only sent
  when both lat and lon are present.
- **Pagination:** `Vessels.Find` and `Vessels.InRadius` are cursor paginated.
  Send `Next` back in the next call; the result's `Next` field mirrors
  `Meta.Next` and is empty on the last page.
- **`InRadiusHistory` is an async report** — it returns a `*Report` with
  `status: "_PENDING_"`. Poll with `Reports.Get(reportID)` until `status` is
  `"_DONE_"`, then download the ZIP from `ResultURL`.
- **402 → `InsufficientCreditsError`**, distinct from 401 auth failure.
- **API key is redacted** from every error message automatically.

## Configuration

Options are validated once, in `NewClient`. An invalid value makes `NewClient`
return a `*DatalasticError` naming the option and the offending value, along
with a nil client; the first failing option wins. Nothing is silently corrected,
and the request path never re-checks configuration.

| Option | Default | Rejected |
|---|---|---|
| `WithTimeout` | 30s | zero or negative |
| `WithHTTPClient` | `&http.Client{Timeout: 30s}` | nil |
| `WithMaxRetries` | 3 | negative |
| `WithBackoffFactor` | 500ms | negative |
| `WithRetryAfterMax` | 60s | negative |
| `WithRetryOnStatus` | `{429}` | any code outside 408, 429, 500-599 |

`IsRetryableStatus(code)` reports which codes `WithRetryOnStatus` accepts.
`WithRetryOnStatus()` with no arguments disables status-based retries.

## Retries

- Wait before zero-based retry attempt `n` is `backoffFactor * 2^n`, capped by
  `WithRetryAfterMax`. The exponent is clamped and the multiplication is
  range-checked, so the delay never overflows.
- A usable `Retry-After` header wins over backoff. Delta-seconds (integer or
  decimal) and HTTP-dates are both parsed; the result is clamped to
  `[0, WithRetryAfterMax]`. An unparseable header falls back to backoff.
- **GET** retries retryable statuses, transport failures (connection refused or
  reset, DNS failure, client timeout), and a body read that ends early
  (`io.ErrUnexpectedEOF` or `io.EOF`, which is what a body shorter than its
  `Content-Length` reports), because GET is idempotent.
- **POST** retries retryable statuses only. A transport failure or a failed body
  read on POST is never repeated: the report job may already have been created.
- Request construction failures (an unparseable URL) are never retried, and
  neither is a certificate rejection: an unknown authority, an invalid
  certificate, or a hostname mismatch cannot be fixed by trying again. Every
  other transport failure is treated as retryable on GET by design — a wasted
  retry is cheaper than missing a transient failure.
- `WithRetryOnStatus` governs status-based retries only. Transport failures and
  failed body reads on GET are governed by `WithMaxRetries` alone, so
  `WithRetryOnStatus()` with no arguments still retries them; `WithMaxRetries(0)`
  disables every retry.
- After the retries are exhausted the error is the same typed error as a single
  attempt would produce. Transport failures report the attempt count:
  `request failed after 4 attempt(s): ...`, and a failed body read reports
  `failed to read response body after 4 attempt(s): ...` while keeping the type
  and status the response implied, so a truncated body on a 429 is still a
  `*RateLimitError`.

## Error Types

| Type | HTTP status |
|---|---|
| `*AuthenticationError` | 401 |
| `*InsufficientCreditsError` | 402 |
| `*NotFoundError` | 404 |
| `*RateLimitError` | 429 |
| `*APIError` | 200 with `meta.success: false`, other HTTP, transport, parse |

All errors unwrap to `*DatalasticError` via `errors.As`. Each typed error also
carries a `StatusCode int` field. Validation errors (missing required params)
are returned as `*DatalasticError` before any network call.

## Response Envelope

All API responses are wrapped:

```json
{"data": <payload>, "meta": {"success": true, "endpoint": "...", "duration": 0.01, "next": "..."}}
```

The SDK parses the envelope once, in a single request path:

1. HTTP status codes at or above 400 map to typed errors.
2. An explicit `meta.success` of `false` becomes an `*APIError` carrying the
   real HTTP status (200) and the message
   `API reported failure: <meta.message>`. A missing meta object or a missing
   `success` key means success.
3. `data` must be present and non-null.

Every returned value exposes the metadata through a `Meta *Meta` field:

- `Success *bool` is nil when the response carried no `success` key.
- `Message`, `Next`, and `Endpoint` are strings; `Duration` is a float64 that
  also accepts a numeric string.
- `Raw map[string]json.RawMessage` holds every key of the meta object verbatim,
  so counters this SDK version does not model yet remain reachable.

`Vessel` declares the field and `VesselPro` and `VesselEstimated` expose it by
promotion, so there is no ambiguous selector. `VesselWithDist` also embeds
`Vessel`; inside an in-radius result its promoted `Meta` stays nil, because the
metadata belongs to the enclosing `VesselInRadiusResult`.
