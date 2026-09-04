// Package datalastic is a Go SDK for the Datalastic Maritime API.
//
// Authentication uses an x-api-key HTTP header on every request.
// The client exposes resource groups for vessels, ports, sea routes, maritime
// intelligence reports, and asynchronous report jobs.
package datalastic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Base URLs for the Datalastic API.
const (
	BaseV0  = "https://api.datalastic.com/api/v0"
	BaseExt = "https://api.datalastic.com/api/ext"
	BaseMR  = "https://api.datalastic.com/api/maritime_reports"
)

// Defaults applied by NewClient before any Option runs.
const (
	defaultTimeout       = 30 * time.Second
	defaultMaxRetries    = 3
	defaultBackoffFactor = 500 * time.Millisecond
	defaultRetryAfterMax = 60 * time.Second
)

// Client is the entry point to the Datalastic API. A Client is safe for
// concurrent use; all of its configuration is fixed at construction.
type Client struct {
	apiKey     string
	baseV0     string
	baseExt    string
	baseMR     string
	httpClient *http.Client

	// Retry configuration, validated once in NewClient and never re-checked
	// in the request path.
	maxRetries    int
	backoffFactor time.Duration
	retryAfterMax time.Duration
	retryStatuses map[int]struct{}

	// sleep and now are seams so tests can exercise the retry loop without
	// waiting and with a fixed clock.
	sleep func(time.Duration)
	now   func() time.Time

	// optErr holds the first option validation failure. NewClient returns it
	// instead of a Client.
	optErr error

	Vessels *VesselsResource
	Ports   *PortsResource
	Routes  *RoutesResource
	Intel   *IntelResource
	Reports *ReportsResource
}

// Option configures a Client. An option given an invalid value records a
// construction error which NewClient returns; options never panic and never
// silently substitute a corrected value.
type Option func(*Client)

// fail records the first option validation error.
func (c *Client) fail(format string, args ...interface{}) {
	if c.optErr == nil {
		c.optErr = &DatalasticError{Message: fmt.Sprintf(format, args...)}
	}
}

// WithTimeout sets a request timeout. It clones the underlying http.Client so a
// shared client is never mutated. The timeout must be greater than zero: a zero
// timeout means "wait forever", which is never what a caller intends when
// setting one explicitly.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d <= 0 {
			c.fail("WithTimeout: timeout must be greater than zero, got %s", d)
			return
		}
		cloned := *c.httpClient
		cloned.Timeout = d
		c.httpClient = &cloned
	}
}

// WithHTTPClient replaces the underlying http.Client. The client must not be
// nil.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc == nil {
			c.fail("WithHTTPClient: http client must not be nil")
			return
		}
		c.httpClient = hc
	}
}

// WithMaxRetries sets how many times a failed request is retried after the
// first attempt. The default is 3. Zero disables retries entirely. Negative
// values are a construction error.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n < 0 {
			c.fail("WithMaxRetries: max retries must not be negative, got %d", n)
			return
		}
		c.maxRetries = n
	}
}

// WithBackoffFactor sets the base delay for exponential backoff. The wait
// before the zero-based retry attempt n is factor * 2^n, capped by the
// Retry-After ceiling. The default is 500ms. Zero means no backoff wait.
// Negative values are a construction error.
func WithBackoffFactor(d time.Duration) Option {
	return func(c *Client) {
		if d < 0 {
			c.fail("WithBackoffFactor: backoff factor must not be negative, got %s", d)
			return
		}
		c.backoffFactor = d
	}
}

// WithRetryAfterMax sets the ceiling for every retry wait, whether it comes
// from a Retry-After header or from exponential backoff. The default is 60s.
// Zero makes every wait zero. Negative values are a construction error.
func WithRetryAfterMax(d time.Duration) Option {
	return func(c *Client) {
		if d < 0 {
			c.fail("WithRetryAfterMax: retry-after ceiling must not be negative, got %s", d)
			return
		}
		c.retryAfterMax = d
	}
}

// WithRetryOnStatus replaces the set of HTTP status codes that trigger a retry.
// The default is 429 alone. Only codes accepted by IsRetryableStatus are
// allowed; anything else is a construction error. Calling it with no arguments
// disables status-based retries.
func WithRetryOnStatus(codes ...int) Option {
	return func(c *Client) {
		set := make(map[int]struct{}, len(codes))
		for _, code := range codes {
			if !IsRetryableStatus(code) {
				c.fail("WithRetryOnStatus: status code %d is not retryable; allowed codes are 408, 429, and 500-599", code)
				return
			}
			set[code] = struct{}{}
		}
		c.retryStatuses = set
	}
}

// withBaseURLs overrides all base URLs. Intended for testing against a local
// server.
func withBaseURLs(base string) Option {
	return func(c *Client) {
		c.baseV0 = base
		c.baseExt = base
		c.baseMR = base
	}
}

// NewClient creates a Client. It returns an error if apiKey is empty or if any
// option was given an invalid value; on error the returned Client is nil.
func NewClient(apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, &DatalasticError{Message: "api key must not be empty"}
	}

	c := &Client{
		apiKey:        apiKey,
		baseV0:        BaseV0,
		baseExt:       BaseExt,
		baseMR:        BaseMR,
		httpClient:    &http.Client{Timeout: defaultTimeout},
		maxRetries:    defaultMaxRetries,
		backoffFactor: defaultBackoffFactor,
		retryAfterMax: defaultRetryAfterMax,
		retryStatuses: map[int]struct{}{http.StatusTooManyRequests: {}},
		sleep:         time.Sleep,
		now:           time.Now,
	}

	for i, opt := range opts {
		if opt == nil {
			c.fail("NewClient: option %d must not be nil", i)
			continue
		}
		opt(c)
	}
	if c.optErr != nil {
		return nil, c.optErr
	}

	c.Vessels = &VesselsResource{client: c}
	c.Ports = &PortsResource{client: c}
	c.Routes = &RoutesResource{client: c}
	c.Intel = &IntelResource{client: c}
	c.Reports = &ReportsResource{client: c}

	return c, nil
}

// envelope is the standard API response wrapper.
type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta Meta            `json:"meta"`
}

// errorBody attempts to parse a structured error message from a response body.
type errorBody struct {
	Data struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
	} `json:"data"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

// attemptResult is the outcome of a single round trip that reached the server.
type attemptResult struct {
	status     int
	body       []byte
	retryAfter string
	readErr    error
}

// do executes a GET request against base+path and returns the data payload and
// the response envelope metadata.
func (c *Client) do(base, path string, params url.Values) (json.RawMessage, *Meta, error) {
	if params == nil {
		params = url.Values{}
	}
	return c.request(http.MethodGet, joinURL(base, path)+"?"+params.Encode(), nil)
}

// post executes a POST request with a JSON body and returns the data payload
// and the response envelope metadata.
func (c *Client) post(base, path string, body map[string]interface{}) (json.RawMessage, *Meta, error) {
	if body == nil {
		body = map[string]interface{}{}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("failed to encode request body: %v", redactKey(err.Error(), c.apiKey))}}
	}
	return c.request(http.MethodPost, joinURL(base, path), payload)
}

// request is the single request path for every endpoint: it performs the round
// trip, applies the retry policy, and parses the response envelope.
//
// GET is idempotent, so transport failures and failed body reads are retried.
// POST is not: the server may have created a report job before the connection
// broke, so for POST only responses with a retryable status are repeated,
// because a status means the server answered without acting.
func (c *Client) request(method, fullURL string, payload []byte) (json.RawMessage, *Meta, error) {
	retryNetwork := method == http.MethodGet

	for attempt := 0; ; attempt++ {
		res, err := c.attempt(method, fullURL, payload)
		if err != nil {
			if retryNetwork && attempt < c.maxRetries && isRetryableNetworkError(err) {
				c.sleep(backoffDelay(c.backoffFactor, attempt, c.retryAfterMax))
				continue
			}
			return nil, nil, &APIError{DatalasticError: DatalasticError{
				Message: fmt.Sprintf("request failed after %d attempt(s): %v", attempt+1, redactKey(err.Error(), c.apiKey)),
			}}
		}

		if attempt < c.maxRetries && c.retriesStatus(res.status) {
			c.sleep(c.retryDelay(attempt, res.retryAfter))
			continue
		}

		if res.readErr != nil {
			// A body that failed or stopped short mid-read is the same class
			// of failure as a broken connection, so it follows the same
			// policy: retried on GET when the cause is retryable, never on
			// POST. attempt() has already closed the failed body.
			if retryNetwork && attempt < c.maxRetries && isRetryableNetworkError(res.readErr) {
				c.sleep(backoffDelay(c.backoffFactor, attempt, c.retryAfterMax))
				continue
			}
			// The message names the read failure, which is the proximate
			// cause, but the type and StatusCode follow the status the server
			// sent: a truncated body on a 429 is still a rate limit, and a
			// caller matching *RateLimitError must not miss it.
			msg := fmt.Sprintf("failed to read response body after %d attempt(s): %v", attempt+1, redactKey(res.readErr.Error(), c.apiKey))
			return nil, nil, statusError(res.status, msg)
		}

		return parseResponse(res.status, res.body, c.apiKey)
	}
}

// attempt performs one round trip. The response body is always drained and
// closed before returning so the connection can be reused by a retry.
func (c *Client) attempt(method, fullURL string, payload []byte) (*attemptResult, error) {
	var body io.Reader
	if payload != nil {
		// A fresh reader per attempt: a consumed one would send an empty body.
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, &nonRetryableError{err}
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(resp.Body)
	return &attemptResult{
		status:     resp.StatusCode,
		body:       raw,
		retryAfter: resp.Header.Get("Retry-After"),
		readErr:    readErr,
	}, nil
}

// parseResponse maps HTTP status codes to typed errors and validates the
// response envelope, returning the data payload and the envelope metadata.
//
// Order of checks: status code, then an explicit meta.success of false (the API
// can report failure on HTTP 200), then the presence of a data payload.
//
// apiKey is only used to redact the key from a decoder error message.
func parseResponse(status int, body []byte, apiKey string) (json.RawMessage, *Meta, error) {
	if status >= 400 {
		return nil, nil, statusError(status, extractErrorMessage(body, status))
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("invalid JSON response: %v", redactKey(err.Error(), apiKey))}, StatusCode: status}
	}
	meta := &env.Meta

	if meta.Success != nil && !*meta.Success {
		msg := "API reported failure"
		if meta.Message != "" {
			msg += ": " + meta.Message
		}
		return nil, nil, &APIError{DatalasticError: DatalasticError{Message: msg}, StatusCode: status}
	}

	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil, nil, &APIError{DatalasticError: DatalasticError{Message: "response missing 'data' field"}, StatusCode: status}
	}

	return env.Data, meta, nil
}

// statusError builds the typed error a status code implies, carrying msg as its
// message. It is the single source of truth for the status-to-type mapping:
// parseResponse uses it for an error response, and request() uses it so that a
// failed body read still surfaces the type the status implies rather than a
// generic *APIError. Any status without a dedicated type, including every 5xx
// and any non-error status, maps to *APIError.
func statusError(status int, msg string) error {
	base := DatalasticError{Message: msg}
	switch status {
	case http.StatusUnauthorized:
		return &AuthenticationError{DatalasticError: base, StatusCode: status}
	case http.StatusPaymentRequired:
		return &InsufficientCreditsError{DatalasticError: base, StatusCode: status}
	case http.StatusNotFound:
		return &NotFoundError{DatalasticError: base, StatusCode: status}
	case http.StatusTooManyRequests:
		return &RateLimitError{DatalasticError: base, StatusCode: status}
	default:
		return &APIError{DatalasticError: base, StatusCode: status}
	}
}

// extractErrorMessage pulls a human-readable message from an error body, falling
// back to a generic status-based message.
func extractErrorMessage(body []byte, status int) string {
	var eb errorBody
	if err := json.Unmarshal(body, &eb); err == nil {
		switch {
		case eb.Data.Message != "":
			return eb.Data.Message
		case eb.Data.Detail != "":
			return eb.Data.Detail
		case eb.Message != "":
			return eb.Message
		case eb.Error != "":
			return eb.Error
		}
	}
	if status >= 400 {
		switch status {
		case http.StatusBadRequest:
			return "bad request"
		case http.StatusUnauthorized:
			return "unauthorized: invalid or expired api key"
		case http.StatusPaymentRequired:
			return "payment required: api credits exhausted"
		case http.StatusNotFound:
			return "not found"
		case http.StatusTooManyRequests:
			return "too many requests: rate limit exceeded"
		case http.StatusInternalServerError:
			return "internal server error"
		}
	}
	return fmt.Sprintf("request failed with status %d", status)
}

// Stat returns API key usage statistics.
func (c *Client) Stat() (*ApiStat, error) {
	raw, meta, err := c.do(c.baseV0, "stat", nil)
	if err != nil {
		return nil, err
	}
	out, err := decodeInto[ApiStat](raw, "stat", c.apiKey)
	if err != nil {
		return nil, err
	}
	out.Meta = meta
	return out, nil
}

// redactKey replaces every occurrence of key in s with "[REDACTED]" so that
// API keys are not leaked in error messages.
func redactKey(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[REDACTED]")
}

// joinURL concatenates a base URL and a path with exactly one separator.
func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// --- shared parameter helpers ---

func addOptionalInt(p url.Values, key string, v *int) {
	if v != nil {
		p.Set(key, strconv.Itoa(*v))
	}
}

func addOptionalFloat(p url.Values, key string, v *float64) {
	if v != nil {
		p.Set(key, strconvFloat(*v))
	}
}

func addString(p url.Values, key, v string) {
	if v != "" {
		p.Set(key, v)
	}
}

func strconvFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
