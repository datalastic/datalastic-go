// Package datalastic is a Go SDK for the Datalastic Maritime API.
//
// Authentication uses an api-key passed as a query parameter on every request.
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

// Client is the entry point to the Datalastic API.
type Client struct {
	apiKey     string
	baseV0     string
	baseExt    string
	baseMR     string
	httpClient *http.Client

	Vessels *VesselsResource
	Ports   *PortsResource
	Routes  *RoutesResource
	Intel   *IntelResource
	Reports *ReportsResource
}

// Option configures a Client.
type Option func(*Client)

// WithTimeout sets a request timeout. It clones the underlying http.Client so a
// shared client is never mutated.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		cloned := *c.httpClient
		cloned.Timeout = d
		c.httpClient = &cloned
	}
}

// WithHTTPClient replaces the underlying http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
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

// NewClient creates a Client. It returns an error if apiKey is empty.
func NewClient(apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, &DatalasticError{Message: "api key must not be empty"}
	}

	c := &Client{
		apiKey:     apiKey,
		baseV0:     BaseV0,
		baseExt:    BaseExt,
		baseMR:     BaseMR,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	for _, opt := range opts {
		opt(c)
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
	Meta struct {
		Duration interface{} `json:"duration"`
		Endpoint string      `json:"endpoint"`
		Success  bool        `json:"success"`
	} `json:"meta"`
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

// do executes a GET request against base+path, appending the api-key query
// parameter, and returns the parsed data payload.
func (c *Client) do(base, path string, params url.Values) (json.RawMessage, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("api-key", c.apiKey)

	fullURL := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
	fullURL += "?" + params.Encode()

	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("request failed: %v", redactKey(err.Error(), c.apiKey))}}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("failed to read response body: %v", err)}, StatusCode: resp.StatusCode}
	}

	return parseResponse(resp.StatusCode, body)
}

// post executes a POST request with a JSON body. The api-key is injected into
// the body map.
func (c *Client) post(base, path string, body map[string]interface{}) (json.RawMessage, error) {
	if body == nil {
		body = map[string]interface{}{}
	}
	body["api-key"] = c.apiKey

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("failed to encode request body: %v", err)}}
	}

	fullURL := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")

	resp, err := c.httpClient.Post(fullURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("request failed: %v", redactKey(err.Error(), c.apiKey))}}
	}
	defer resp.Body.Close()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("failed to read response body: %v", err)}, StatusCode: resp.StatusCode}
	}

	return parseResponse(resp.StatusCode, rb)
}

// parseResponse maps HTTP status codes to typed errors and validates the
// response envelope.
func parseResponse(status int, body []byte) (json.RawMessage, error) {
	if status >= 400 {
		msg := extractErrorMessage(body, status)
		base := DatalasticError{Message: msg}
		switch status {
		case http.StatusUnauthorized:
			return nil, &AuthenticationError{DatalasticError: base, StatusCode: status}
		case http.StatusPaymentRequired:
			return nil, &InsufficientCreditsError{DatalasticError: base, StatusCode: status}
		case http.StatusNotFound:
			return nil, &NotFoundError{DatalasticError: base, StatusCode: status}
		case http.StatusTooManyRequests:
			return nil, &RateLimitError{DatalasticError: base, StatusCode: status}
		default:
			return nil, &APIError{DatalasticError: base, StatusCode: status}
		}
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("invalid JSON response: %v", err)}, StatusCode: status}
	}

	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil, &APIError{DatalasticError: DatalasticError{Message: "response missing 'data' field"}, StatusCode: status}
	}

	return env.Data, nil
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
	raw, err := c.do(c.baseV0, "stat", nil)
	if err != nil {
		return nil, err
	}
	var out ApiStat
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &APIError{DatalasticError: DatalasticError{Message: fmt.Sprintf("failed to decode stat: %v", err)}}
	}
	return &out, nil
}

// redactKey replaces every occurrence of key (raw and URL-encoded) in s with
// "[REDACTED]" so that API keys are not leaked in error messages.
func redactKey(s, key string) string {
	if key == "" {
		return s
	}
	s = strings.ReplaceAll(s, key, "[REDACTED]")
	if encoded := url.QueryEscape(key); encoded != key {
		s = strings.ReplaceAll(s, encoded, "[REDACTED]")
	}
	return s
}

// --- shared parameter helpers ---

func addOptionalInt(p url.Values, key string, v *int) {
	if v != nil {
		p.Set(key, fmt.Sprintf("%d", *v))
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
