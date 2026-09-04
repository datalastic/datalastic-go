package datalastic

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxBackoffExponent caps the shift used when computing exponential backoff so
// the delay calculation can never overflow, no matter how many attempts are
// configured.
const maxBackoffExponent = 32

// IsRetryableStatus reports whether an HTTP status code may be configured for
// automatic retries. Only 408 (Request Timeout), 429 (Too Many Requests), and
// the 5xx server error range qualify: every other status describes a condition
// that repeating the request cannot fix.
func IsRetryableStatus(code int) bool {
	switch {
	case code == http.StatusRequestTimeout:
		return true
	case code == http.StatusTooManyRequests:
		return true
	case code >= 500 && code <= 599:
		return true
	default:
		return false
	}
}

// nonRetryableError marks a failure that happened before the request left the
// process (for example an unparseable URL). Repeating it would fail the same
// way, so it is never retried.
type nonRetryableError struct{ err error }

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

// isRetryableNetworkError reports whether a failure that happened while sending
// the request or reading its body is worth repeating.
//
// The check is deliberately broad rather than selective. Almost every
// transport-level failure http.Client reports satisfies net.Error, including
// ones that are certainly permanent, and this returns true for them. That is
// the intended trade: GET is idempotent, so a wasted retry costs one round trip
// and a bounded wait, while refusing to retry a failure that was in fact
// transient costs the caller the whole request. Narrowing the net.Error branch
// to Timeout() alone would drop connection resets and DNS blips, which is the
// worse mistake. Only failures that cannot possibly succeed on a second attempt
// are carved out.
//
// Classification, in order:
//  1. Request construction failures are never retryable: the request never left
//     the process, so repeating it fails identically.
//  2. An error carrying context.Canceled or context.DeadlineExceeded itself is
//     never retryable. See hasContextSentinel.
//  3. A certificate rejection is never retryable. An untrusted authority, an
//     invalid certificate, or a hostname mismatch is a configuration fact, not
//     a transient condition, yet it reaches here wrapped in a *url.Error, which
//     does satisfy net.Error. See hasPermanentTLSError.
//  4. Anything else satisfying net.Error is retryable: connection refused,
//     connection reset, DNS failure, and the *url.Error that http.Client
//     returns when its own Timeout fires, but also any other transport failure
//     the standard library reports that way.
//  5. A response body that ended early is retryable: io.ErrUnexpectedEOF, which
//     is what a body shorter than its Content-Length reports, and a bare io.EOF.
//     Neither is a net.Error, and the first arrives unwrapped.
//
// Only GET uses this classification, for both a transport failure and a failed
// body read. POST is not idempotent and is never retried on either.
func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var nre *nonRetryableError
	if errors.As(err, &nre) {
		return false
	}
	if hasContextSentinel(err) {
		return false
	}
	if hasPermanentTLSError(err) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
}

// hasPermanentTLSError reports whether err carries a certificate verification
// failure, which no number of retries can fix.
//
// crypto/x509 returns these three as values, but a transport is free to wrap a
// pointer to one, so both forms are matched.
func hasPermanentTLSError(err error) bool {
	var (
		invalid   x509.CertificateInvalidError
		authority x509.UnknownAuthorityError
		hostname  x509.HostnameError
	)
	if errors.As(err, &invalid) || errors.As(err, &authority) || errors.As(err, &hostname) {
		return true
	}
	var (
		invalidPtr   *x509.CertificateInvalidError
		authorityPtr *x509.UnknownAuthorityError
		hostnamePtr  *x509.HostnameError
	)
	return errors.As(err, &invalidPtr) || errors.As(err, &authorityPtr) || errors.As(err, &hostnamePtr)
}

// hasContextSentinel reports whether err carries context.Canceled or
// context.DeadlineExceeded itself.
//
// This is a defensive guard for a custom transport installed with
// WithHTTPClient that surfaces a context sentinel directly: repeating a request
// whose context is already done would fail immediately anyway. No SDK method
// takes a context.Context, so the standard transport does not reach it.
//
// Identity, not errors.Is, is what makes the guard safe. The error http.Client
// reports when its own Timeout fires answers true to
// errors.Is(err, context.DeadlineExceeded) even though no context of the
// caller's is involved; it wraps its own timeout type rather than the sentinel,
// so walking the chain for the sentinel value keeps client timeouts retryable.
func hasContextSentinel(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if e == context.Canceled || e == context.DeadlineExceeded {
			return true
		}
	}
	return false
}

// backoffDelay returns the wait before the zero-based retry attempt n:
// factor * 2^min(n, maxBackoffExponent), capped at ceiling.
//
// The exponent is clamped and the multiplication is range-checked against the
// ceiling before it happens, so the result can never overflow into a negative
// duration.
func backoffDelay(factor time.Duration, attempt int, ceiling time.Duration) time.Duration {
	if factor <= 0 || ceiling <= 0 {
		return 0
	}
	exp := attempt
	if exp < 0 {
		exp = 0
	}
	if exp > maxBackoffExponent {
		exp = maxBackoffExponent
	}
	// factor<<exp <= ceiling is checked without performing the shift.
	if factor > ceiling>>uint(exp) {
		return ceiling
	}
	return factor << uint(exp)
}

// maxRetryAfterSeconds is the largest delta-seconds value that still converts
// to a time.Duration without overflowing.
const maxRetryAfterSeconds = float64(math.MaxInt64) / float64(time.Second)

// parseRetryAfter interprets a Retry-After header value, which may be either
// delta-seconds or an HTTP-date. It reports false when the header is absent or
// cannot be interpreted, in which case the caller falls back to backoff.
//
// Negative, NaN, and infinite delta-seconds are rejected. A date in the past
// yields a zero delay rather than a negative one.
func parseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(header, 64); err == nil {
		if math.IsNaN(secs) || math.IsInf(secs, 0) || secs < 0 {
			return 0, false
		}
		if secs >= maxRetryAfterSeconds {
			return time.Duration(math.MaxInt64), true
		}
		return time.Duration(secs * float64(time.Second)), true
	}
	if t, err := http.ParseTime(header); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

// clampDuration confines d to [0, ceiling].
func clampDuration(d, ceiling time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > ceiling {
		return ceiling
	}
	return d
}

// retryDelay returns how long to wait before the zero-based retry attempt n,
// preferring a usable Retry-After header over exponential backoff. Every wait
// is capped by the configured Retry-After ceiling.
func (c *Client) retryDelay(attempt int, retryAfter string) time.Duration {
	if d, ok := parseRetryAfter(retryAfter, c.now()); ok {
		return clampDuration(d, c.retryAfterMax)
	}
	return backoffDelay(c.backoffFactor, attempt, c.retryAfterMax)
}

// retriesStatus reports whether the client is configured to retry a status.
func (c *Client) retriesStatus(code int) bool {
	_, ok := c.retryStatuses[code]
	return ok
}
