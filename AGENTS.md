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

## Key Conventions

- **Auth is an HTTP header.** Both GET and POST requests set `x-api-key` on the request header. The key never appears in the URL query string or JSON body.
- **Three base URLs:** `BaseV0` (`/api/v0`) for core endpoints; `BaseExt` (`/api/ext`) for `Estimated` and `Calculate`; `BaseMR` (`/api/maritime_reports`) for all Intel methods.
- **Bulk uses repeated params:** `vessel_bulk` sends multiple `mmsi=` / `imo=` / `uuid=` values via `url.Values.Add`.
- **`VesselFindParams.VesselType` maps to the `type` query key** — not `vessel_type`.
- **Find/search guards:** `Vessels.Find` requires at least one of name, type, country, or a range bound (Fuzzy/Next alone are rejected). `Ports.Find` requires at least one of name, UUID, port type, country, UNLOCODE, or lat+lon.
- **Pointer semantics:** optional params and nullable response fields are `*T`. Use `addOptionalInt` / `addOptionalFloat` helpers when building query values.
- **`InRadiusHistory` is an async report** — it returns a `*Report` with `status: "_PENDING_"`. Poll with `Reports.Get(reportID)` until `status` is `"_DONE_"`, then download the ZIP from `ResultURL`.
- **402 → `InsufficientCreditsError`**, distinct from 401 auth failure.
- **API key is redacted** from connection error messages automatically.

## Error Types

| Type | HTTP status |
|---|---|
| `*AuthenticationError` | 401 |
| `*InsufficientCreditsError` | 402 |
| `*NotFoundError` | 404 |
| `*RateLimitError` | 429 |
| `*APIError` | other HTTP / transport / parse |

All errors unwrap to `*DatalasticError` via `errors.As`. Each typed error also carries a `StatusCode int` field. Validation errors (missing required params) are returned as `*DatalasticError` before any network call.

## Response Envelope

All API responses are wrapped:
```json
{"data": <payload>, "meta": {"success": true, "endpoint": "...", "duration": 0.01}}
```
The SDK validates that `data` is present and non-null before decoding.
