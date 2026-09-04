# Changelog

All notable changes to the Datalastic Go SDK are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-09-04

First tagged release. Brings the Go SDK to contract parity with the Python and
Node SDKs.

### Added

- **Automatic retries.** Rate-limited and transient failures are now retried
  with exponential backoff instead of failing immediately. The server's
  `Retry-After` header is honoured when present. Tune it with
  `WithMaxRetries`, `WithBackoffFactor`, `WithRetryAfterMax` and
  `WithRetryOnStatus`, or set `WithMaxRetries(0)` to opt out.
- **Response metadata.** Every result carries a `Meta` field with the API's
  `success`, `message`, `endpoint` and `duration` values, plus a `Raw` map
  holding every meta key verbatim so new API fields are readable without an SDK
  upgrade. Slice-returning methods gained a `...WithMeta` sibling
  (`Ports.FindWithMeta`, `Intel.OwnershipWithMeta`, and so on) that returns the
  metadata alongside the records.
- **Pagination tokens.** `VesselFindResult.Next` and `VesselInRadiusResult.Next`
  expose the API's continuation token, so large result sets can be paged
  through.
- **`IncludeNullType` filter** on vessel search, mapping to the API's `_empty_`
  parameter, for matching vessels with no declared type.

### Changed

- **The API key is sent as an `x-api-key` header** rather than in the URL query
  string, so it no longer appears in server logs, proxy logs or browser history.
- **The API key is redacted from every error message**, including transport,
  decode and parse failures.
- **A `success: false` response is now an error.** The API can return HTTP 200
  with a failure in the envelope; this previously surfaced as a confusing
  "missing 'data' field" error or, worse, passed as success. It now returns a
  typed `*APIError` carrying the API's own message.
- **Invalid client options fail at construction.** `NewClient` validates its
  options and returns an error instead of handing back a client that misbehaves
  later. Rejected values include a non-positive timeout, a nil HTTP client and a
  negative retry count.

[0.2.0]: https://github.com/datalastic/datalastic-go/releases/tag/v0.2.0
