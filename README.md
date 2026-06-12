# Datalastic Go SDK: Real-Time Vessel Tracking for Go Applications

The **Datalastic Go SDK** is the official Go library for the [Datalastic Maritime API](https://datalastic.com). It gives Go applications direct access to AIS vessel tracking data, port information, sea route calculations, and maritime intelligence reports. The SDK uses only the Go standard library with no external dependencies.

## Why Use This SDK?

- **Zero dependencies.** The SDK builds with `stdlib` only, which means no dependency conflicts, no supply-chain risk, and straightforward vendoring.
- **Full API coverage.** Every Datalastic endpoint is available, from basic AIS position lookups to async maritime intelligence reports.
- **Typed errors.** Each HTTP error maps to a specific Go type so you can handle authentication failures, credit exhaustion, and rate limits with `errors.As` without parsing strings.

## Features

- Vessel tracking (basic, pro, bulk, in-radius, history, info, find, satellite-estimated)
- Port lookup and detail (with terminals)
- Sea route calculation
- Maritime intelligence reports (dry dock, casualty, inspections, sales/purchases/demolitions, ownership, class society, engine, companies)
- Asynchronous report jobs
- Typed error hierarchy with `errors.As` support
- Configurable timeout and HTTP client

## Installation

```bash
go get github.com/datalastic/datalastic-go
```

```go
import datalastic "github.com/datalastic/datalastic-go"
```

Requires Go 1.21 or later.

## Authentication

Pass your API key once at construction. The SDK injects it automatically as an `x-api-key` HTTP header on every request.

```go
client, err := datalastic.NewClient("YOUR_API_KEY")
if err != nil {
    log.Fatal(err)
}
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"

    datalastic "github.com/datalastic/datalastic-go"
)

func main() {
    client, err := datalastic.NewClient("YOUR_API_KEY")
    if err != nil {
        log.Fatal(err)
    }

    vessel, err := client.Vessels.Get(datalastic.VesselParams{MMSI: "636019825"})
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("%s is at %.4f, %.4f doing %.1f knots\n",
        vessel.Name, vessel.Lat, vessel.Lon, vessel.Speed)
}
```

## Configuration

Use functional options to adjust timeout or supply a custom HTTP client.

```go
import "time"

// Custom timeout (clones the underlying HTTP client; never mutates a shared one).
client, _ := datalastic.NewClient("KEY", datalastic.WithTimeout(15*time.Second))

// Supply your own *http.Client.
hc := &http.Client{ /* transport, proxy, etc. */ }
client, _ = datalastic.NewClient("KEY", datalastic.WithHTTPClient(hc))
```

## Resources

### Account

```go
stat, err := client.Stat()
// stat.RequestsRemaining, stat.KeyStatus, ...
```

### Vessels

The vessels resource covers the full range of AIS data available from the Datalastic API, from live position to static specifications.

```go
// Basic AIS position
v, err := client.Vessels.Get(datalastic.VesselParams{IMO: "9379785"})

// Extended data: ETA, ATD, ports, draught
pro, err := client.Vessels.Pro(datalastic.VesselParams{UUID: "..."})

// Multiple vessels at once (repeated query params)
bulk, err := client.Vessels.Bulk(datalastic.VesselBulkParams{
    MMSI: []string{"636019825", "538008251"},
})

// Vessels within a radius (km) of a point or port. Radius is required.
lat, lon := 51.95, 4.14
inrad, err := client.Vessels.InRadius(datalastic.VesselInRadiusParams{
    Lat:    &lat,
    Lon:    &lon,
    Radius: 50,
})

// Historical track
days := 7
hist, err := client.Vessels.History(datalastic.VesselHistoryParams{
    IMO:  "9379785",
    Days: &days,
})

// Static specifications
info, err := client.Vessels.Info(datalastic.VesselParams{IMO: "9379785"})

// Search the static database. At least one real search parameter is required;
// Fuzzy and Next do not count. VesselType maps to the API "type" parameter.
results, err := client.Vessels.Find(datalastic.VesselFindParams{
    VesselType: "cargo",
    CountryISO: "NL",
})

// Satellite-estimated position (extended endpoint)
est, err := client.Vessels.Estimated(datalastic.VesselParams{IMO: "9379785"})
```

### Ports

At least one search parameter is required for `Find`. `Get` returns full port detail including terminals.

```go
// Search ports (at least one search parameter required)
ports, err := client.Ports.Find(datalastic.PortFindParams{CountryISO: "NL"})

// Full detail including terminals
lat, lon := 51.95, 4.14
detail, err := client.Ports.Get(datalastic.PortGetParams{Unlocode: "NLRTM"})
_ = lat
_ = lon
for _, term := range detail.Terminals {
    fmt.Println(term.TerminalName)
}
```

### Routes

Calculate a navigable sea route between two coordinates and read the total distance in nautical miles.

```go
latFrom, lonFrom := 51.95, 4.14
latTo, lonTo := 1.29, 103.85
route, err := client.Routes.Calculate(datalastic.RouteParams{
    LatFrom: &latFrom, LonFrom: &lonFrom,
    LatTo:   &latTo,   LonTo:   &lonTo,
})
fmt.Printf("Total distance: %.1f nm\n", route.Route.Properties.TotalDist)
```

### Maritime Intelligence

Each method returns structured data for a specific intelligence domain. All accept an IMO or company IMO as the primary identifier.

```go
dryDock, err := client.Intel.DryDock(datalastic.IntelDryDockParams{IMO: "9379785"})
casualties, err := client.Intel.Casualties(datalastic.IntelDateRangeParams{
    IMO: "9379785", From: "2023-01-01", To: "2024-01-01",
})
inspections, err := client.Intel.Inspections(datalastic.IntelDateRangeParams{IMO: "9379785"})
spd, err := client.Intel.SPD(datalastic.IntelDateRangeParams{IMO: "9379785"})
ownership, err := client.Intel.Ownership(datalastic.OwnershipParams{IMO: "9379785"})
classSoc, err := client.Intel.ClassSociety(datalastic.ClassSocietyParams{IMO: "9379785"})
engine, err := client.Intel.Engine(datalastic.EngineParams{IMO: "9379785"})
companies, err := client.Intel.Companies(datalastic.CompanyParams{CompanyIMO: "1234567"})
```

### Reports

Reports are asynchronous. Submit a job, then poll with `Get` until the status is `_DONE_`.

```go
// Submit an async job (api-key is sent in the x-api-key header)
report, err := client.Reports.Submit(datalastic.ReportSubmitParams{
    ReportType: "fleet_movement",
    Extra:      map[string]interface{}{"imo": "9379785"},
})

// Historical area scan: all vessels that passed through a radius over a date range.
// Returns immediately with status "_PENDING_"; poll until "_DONE_", then download
// the ZIP from report.ResultURL.
lat, lon := 51.8951, 4.3973
report, err = client.Reports.InRadiusHistory(datalastic.InRadiusHistoryParams{
    Lat:    &lat,
    Lon:    &lon,
    Radius: 10,
    From:   "2024-01-01",
    To:     "2024-01-07",
})

// Poll a single job
report, err = client.Reports.Get(report.ReportID)

// List all jobs for the key
all, err := client.Reports.ListAll()
```

## Error Handling

All errors implement `error` and unwrap to the base `*DatalasticError`. Use `errors.As` to discriminate by type. Each typed error carries the HTTP `StatusCode`.

| HTTP status | Error type |
|---|---|
| 401 | `*AuthenticationError` |
| 402 | `*InsufficientCreditsError` |
| 404 | `*NotFoundError` |
| 429 | `*RateLimitError` |
| other 4xx/5xx, transport, parse | `*APIError` |

```go
import "errors"

v, err := client.Vessels.Get(datalastic.VesselParams{IMO: "9379785"})
if err != nil {
    var authErr *datalastic.AuthenticationError
    var creditErr *datalastic.InsufficientCreditsError
    var rateErr *datalastic.RateLimitError
    switch {
    case errors.As(err, &authErr):
        log.Fatal("invalid or expired API key")
    case errors.As(err, &creditErr):
        log.Fatal("API credits exhausted")
    case errors.As(err, &rateErr):
        log.Fatal("rate limited, back off and retry")
    default:
        var base *datalastic.DatalasticError
        errors.As(err, &base)
        log.Fatalf("request failed: %s", base.Message)
    }
}
```

Validation errors (for example, calling `Vessels.Get` with no identifier) are returned as `*DatalasticError` before any network call is made.

## Development

```bash
go test ./...
go test -cover ./...
go vet ./...
gofmt -l .
```

## License

MIT. See [LICENSE](LICENSE).
