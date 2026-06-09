package datalastic

// DatalasticError is the base type for all SDK errors.
type DatalasticError struct{ Message string }

func (e *DatalasticError) Error() string { return e.Message }

// AuthenticationError is returned for HTTP 401.
type AuthenticationError struct {
	DatalasticError
	StatusCode int
}

func (e *AuthenticationError) Unwrap() error { return &e.DatalasticError }

// InsufficientCreditsError is returned for HTTP 402 (credits exhausted).
type InsufficientCreditsError struct {
	DatalasticError
	StatusCode int
}

func (e *InsufficientCreditsError) Unwrap() error { return &e.DatalasticError }

// NotFoundError is returned for HTTP 404.
type NotFoundError struct {
	DatalasticError
	StatusCode int
}

func (e *NotFoundError) Unwrap() error { return &e.DatalasticError }

// RateLimitError is returned for HTTP 429.
type RateLimitError struct {
	DatalasticError
	StatusCode int
}

func (e *RateLimitError) Unwrap() error { return &e.DatalasticError }

// APIError is returned for all other HTTP/transport errors.
type APIError struct {
	DatalasticError
	StatusCode int
}

func (e *APIError) Unwrap() error { return &e.DatalasticError }
