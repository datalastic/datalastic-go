package datalastic

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixedNow is the clock every retry test runs against, so HTTP-date handling is
// deterministic.
var fixedNow = time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

// sleepLog records the retry waits instead of performing them. No retry test
// ever waits for real.
type sleepLog struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (s *sleepLog) record(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waits = append(s.waits, d)
}

func (s *sleepLog) all() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]time.Duration, len(s.waits))
	copy(out, s.waits)
	return out
}

// scriptStep is one scripted response from the test server.
type scriptStep struct {
	status     int
	body       string
	retryAfter string
}

// scriptedServer replies with steps[i] for attempt i, repeating the last step
// once the script runs out. The returned func reports how many requests
// arrived.
func scriptedServer(t *testing.T, steps ...scriptStep) (*httptest.Server, func() int) {
	t.Helper()
	if len(steps) == 0 {
		t.Fatal("scriptedServer needs at least one step")
	}
	var mu sync.Mutex
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		i := count
		count++
		mu.Unlock()

		step := steps[len(steps)-1]
		if i < len(steps) {
			step = steps[i]
		}
		if step.retryAfter != "" {
			w.Header().Set("Retry-After", step.retryAfter)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(step.status)
		_, _ = io.WriteString(w, step.body)
	}))
	t.Cleanup(srv.Close)
	return srv, func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
}

// retryClient builds a Client with the sleep and clock seams replaced.
func retryClient(t *testing.T, baseURL string, opts ...Option) (*Client, *sleepLog) {
	t.Helper()
	all := append([]Option{withBaseURLs(baseURL)}, opts...)
	c, err := NewClient("test-key", all...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	log := &sleepLog{}
	c.sleep = log.record
	c.now = func() time.Time { return fixedNow }
	return c, log
}

func assertWaits(t *testing.T, got []time.Duration, want ...time.Duration) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("waits = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wait %d = %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}
}

// --- Status retries ---

func TestRetry_GET_429ThenSuccess(t *testing.T) {
	srv, count := scriptedServer(t,
		scriptStep{status: 429, body: `{"error":"slow down"}`},
		scriptStep{status: 200, body: dataEnvelope(`{"user_id":"u1"}`)},
	)
	c, waits := retryClient(t, srv.URL)

	stat, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat should succeed after one retry: %v", err)
	}
	if stat.UserID != "u1" {
		t.Fatalf("unexpected stat: %+v", stat)
	}
	if count() != 2 {
		t.Fatalf("requests = %d, want 2", count())
	}
	assertWaits(t, waits.all(), 500*time.Millisecond)
}

func TestRetry_GET_429Exhausted(t *testing.T) {
	srv, count := scriptedServer(t, scriptStep{status: 429, body: `{"error":"slow down"}`})
	c, waits := retryClient(t, srv.URL)

	_, err := c.Stat()
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.StatusCode != 429 {
		t.Fatalf("expected RateLimitError(429), got %T: %v", err, err)
	}
	if count() != 4 {
		t.Fatalf("requests = %d, want 4 (1 + 3 retries)", count())
	}
	assertWaits(t, waits.all(), 500*time.Millisecond, time.Second, 2*time.Second)
}

func TestRetry_MaxRetriesZero(t *testing.T) {
	srv, count := scriptedServer(t, scriptStep{status: 429, body: `{}`})
	c, waits := retryClient(t, srv.URL, WithMaxRetries(0))

	if _, err := c.Stat(); err == nil {
		t.Fatal("expected rate limit error")
	}
	if count() != 1 {
		t.Fatalf("requests = %d, want 1", count())
	}
	if got := waits.all(); len(got) != 0 {
		t.Fatalf("waits = %v, want none", got)
	}
}

func TestRetry_500NotRetriedByDefault(t *testing.T) {
	srv, count := scriptedServer(t, scriptStep{status: 500, body: `{}`})
	c, waits := retryClient(t, srv.URL)

	_, err := c.Stat()
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode != 500 {
		t.Fatalf("expected APIError(500), got %T: %v", err, err)
	}
	if count() != 1 {
		t.Fatalf("requests = %d, want 1: 500 is not retried by default", count())
	}
	if got := waits.all(); len(got) != 0 {
		t.Fatalf("waits = %v, want none", got)
	}
}

func TestRetry_ServerErrorsRetriedWhenOptedIn(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, count := scriptedServer(t,
				scriptStep{status: status, body: `{}`},
				scriptStep{status: 200, body: dataEnvelope(`{"user_id":"u1"}`)},
			)
			c, waits := retryClient(t, srv.URL, WithRetryOnStatus(429, 500, 502, 503, 504))

			if _, err := c.Stat(); err != nil {
				t.Fatalf("status %d should be retried: %v", status, err)
			}
			if count() != 2 {
				t.Fatalf("requests = %d, want 2", count())
			}
			assertWaits(t, waits.all(), 500*time.Millisecond)
		})
	}
}

func TestRetry_NonRetryableStatusesReturnedImmediately(t *testing.T) {
	cases := []struct {
		status int
		check  func(error) bool
	}{
		{401, func(err error) bool { var e *AuthenticationError; return errors.As(err, &e) }},
		{402, func(err error) bool { var e *InsufficientCreditsError; return errors.As(err, &e) }},
		{404, func(err error) bool { var e *NotFoundError; return errors.As(err, &e) }},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			// 429 is in the retry set, but the server answers with a status
			// that must never be retried.
			srv, count := scriptedServer(t, scriptStep{status: tc.status, body: `{}`})
			c, waits := retryClient(t, srv.URL, WithRetryOnStatus(429, 500))

			_, err := c.Stat()
			if err == nil || !tc.check(err) {
				t.Fatalf("status %d: wrong error %T: %v", tc.status, err, err)
			}
			if count() != 1 {
				t.Fatalf("status %d: requests = %d, want 1", tc.status, count())
			}
			if got := waits.all(); len(got) != 0 {
				t.Fatalf("status %d: waits = %v, want none", tc.status, got)
			}
		})
	}
}

// --- Retry-After ---

func TestRetry_RetryAfterHeader(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"integer seconds", "2", 2 * time.Second},
		{"decimal seconds", "1.5", 1500 * time.Millisecond},
		{"huge value clamped to ceiling", "999999", 60 * time.Second},
		{"http date in the future", fixedNow.Add(3 * time.Second).Format(http.TimeFormat), 3 * time.Second},
		{"http date in the past", fixedNow.Add(-time.Hour).Format(http.TimeFormat), 0},
		{"unparseable falls back to backoff", "very soon", 500 * time.Millisecond},
		{"negative falls back to backoff", "-5", 500 * time.Millisecond},
		{"absent falls back to backoff", "", 500 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, count := scriptedServer(t,
				scriptStep{status: 429, body: `{}`, retryAfter: tc.header},
				scriptStep{status: 200, body: dataEnvelope(`{"user_id":"u1"}`)},
			)
			c, waits := retryClient(t, srv.URL)

			if _, err := c.Stat(); err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if count() != 2 {
				t.Fatalf("requests = %d, want 2", count())
			}
			assertWaits(t, waits.all(), tc.want)
		})
	}
}

func TestRetry_RetryAfterClampedByConfiguredCeiling(t *testing.T) {
	srv, _ := scriptedServer(t,
		scriptStep{status: 429, body: `{}`, retryAfter: "300"},
		scriptStep{status: 200, body: dataEnvelope(`{"user_id":"u1"}`)},
	)
	c, waits := retryClient(t, srv.URL, WithRetryAfterMax(5*time.Second))

	if _, err := c.Stat(); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	assertWaits(t, waits.all(), 5*time.Second)
}

func TestRetry_ZeroCeilingMakesEveryWaitZero(t *testing.T) {
	srv, count := scriptedServer(t,
		scriptStep{status: 429, body: `{}`, retryAfter: "30"},
		scriptStep{status: 200, body: dataEnvelope(`{"user_id":"u1"}`)},
	)
	c, waits := retryClient(t, srv.URL, WithRetryAfterMax(0))

	if _, err := c.Stat(); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if count() != 2 {
		t.Fatalf("requests = %d, want 2", count())
	}
	assertWaits(t, waits.all(), 0)
}

// --- Network errors ---

// deadServerURL returns the URL of a server that has already been shut down, so
// connecting to it fails at the transport level.
func deadServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	u := srv.URL
	srv.Close()
	return u
}

func TestRetry_GETNetworkErrorRetried(t *testing.T) {
	c, waits := retryClient(t, deadServerURL(t))

	_, err := c.Stat()
	if err == nil {
		t.Fatal("expected connection error")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if !strings.Contains(err.Error(), "after 4 attempt(s)") {
		t.Fatalf("error should report the attempt count, got %q", err.Error())
	}
	assertWaits(t, waits.all(), 500*time.Millisecond, time.Second, 2*time.Second)
}

func TestRetry_POSTNetworkErrorNotRetried(t *testing.T) {
	c, waits := retryClient(t, deadServerURL(t))

	_, err := c.Reports.Submit(ReportSubmitParams{ReportType: "fleet"})
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "after 1 attempt(s)") {
		t.Fatalf("POST must not retry a transport failure, got %q", err.Error())
	}
	if got := waits.all(); len(got) != 0 {
		t.Fatalf("waits = %v, want none", got)
	}
}

// truncatedBodyServer promises 1000 bytes, writes a few, then aborts the
// connection, so every attempt fails with "unexpected EOF" while reading the
// body rather than while connecting. The returned func reports how many
// requests arrived.
func truncatedBodyServer(t *testing.T) (*httptest.Server, func() int) {
	t.Helper()
	return truncatedBodyServerStatus(t, 200)
}

// truncatedBodyServerStatus is truncatedBodyServer with an explicit status, so
// a test can pair a truncated body with a status that maps to a typed error.
func truncatedBodyServerStatus(t *testing.T, status int) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()

		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"data":`)
		_ = http.NewResponseController(w).Flush()
		// ErrAbortHandler drops the connection without logging a stack trace.
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)
	return srv, func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
}

func TestRetry_TruncatedBodyRetriedOnGet(t *testing.T) {
	srv, count := truncatedBodyServer(t)
	c, waits := retryClient(t, srv.URL, WithMaxRetries(3))

	_, err := c.Stat()
	if err == nil {
		t.Fatal("expected a body read error")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("error should name the truncation, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "after 4 attempt(s)") {
		t.Fatalf("error should report the attempt count, got %q", err.Error())
	}
	if count() != 4 {
		t.Fatalf("requests = %d, want 4 (1 + 3 retries)", count())
	}
	assertWaits(t, waits.all(), 500*time.Millisecond, time.Second, 2*time.Second)
}

func TestRetry_TruncatedBodyNotRetriedOnPost(t *testing.T) {
	srv, count := truncatedBodyServer(t)
	c, waits := retryClient(t, srv.URL, WithMaxRetries(3))

	_, err := c.Reports.Submit(ReportSubmitParams{ReportType: "fleet"})
	if err == nil {
		t.Fatal("expected a body read error")
	}
	if !strings.Contains(err.Error(), "after 1 attempt(s)") {
		t.Fatalf("POST must not retry a failed body read, got %q", err.Error())
	}
	if count() != 1 {
		t.Fatalf("requests = %d, want 1", count())
	}
	if got := waits.all(); len(got) != 0 {
		t.Fatalf("waits = %v, want none", got)
	}
}

func TestRetry_TruncatedBodyKeepsTypedStatusError(t *testing.T) {
	// A truncated body does not erase the status the server sent. After the
	// retries are exhausted the caller must still get the type the status
	// implies, or errors.As for *RateLimitError silently misses a 429.
	cases := []struct {
		name   string
		status int
		check  func(error) (int, bool)
	}{
		{"429 stays a rate limit", 429, func(err error) (int, bool) {
			var e *RateLimitError
			ok := errors.As(err, &e)
			if !ok {
				return 0, false
			}
			return e.StatusCode, true
		}},
		{"404 stays a not found", 404, func(err error) (int, bool) {
			var e *NotFoundError
			ok := errors.As(err, &e)
			if !ok {
				return 0, false
			}
			return e.StatusCode, true
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, count := truncatedBodyServerStatus(t, tc.status)
			c, waits := retryClient(t, srv.URL, WithMaxRetries(2))

			_, err := c.Stat()
			if err == nil {
				t.Fatal("expected an error")
			}
			gotStatus, ok := tc.check(err)
			if !ok {
				t.Fatalf("status %d: wrong error type %T: %v", tc.status, err, err)
			}
			if gotStatus != tc.status {
				t.Fatalf("StatusCode = %d, want %d", gotStatus, tc.status)
			}
			if !strings.Contains(err.Error(), "failed to read response body after 3 attempt(s)") {
				t.Fatalf("message should name the read failure and the attempt count, got %q", err.Error())
			}
			if count() != 3 {
				t.Fatalf("requests = %d, want 3 (1 + 2 retries)", count())
			}
			assertWaits(t, waits.all(), 500*time.Millisecond, time.Second)
		})
	}
}

func TestRetry_CertificateFailureNotRetriedOnGet(t *testing.T) {
	// A self-signed certificate that the client does not trust: the handshake
	// fails the same way every time, so retrying only wastes round trips.
	var mu sync.Mutex
	count := 0
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
	}))
	// The handshake failure is the point of the test, not something to log.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	t.Cleanup(srv.Close)

	c, waits := retryClient(t, srv.URL)
	_, err := c.Stat()
	if err == nil {
		t.Fatal("expected a certificate error")
	}
	if !strings.Contains(err.Error(), "after 1 attempt(s)") {
		t.Fatalf("a certificate failure must not be retried, got %q", err.Error())
	}
	if got := waits.all(); len(got) != 0 {
		t.Fatalf("waits = %v, want none", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if count != 0 {
		t.Fatalf("handler ran %d times, want 0: the handshake never completes", count)
	}
}

func TestRetry_POST429Retried(t *testing.T) {
	srv, count := scriptedServer(t,
		scriptStep{status: 429, body: `{}`},
		scriptStep{status: 200, body: dataEnvelope(`{"report_id":"r-1","status":"queued"}`)},
	)
	c, waits := retryClient(t, srv.URL)

	rep, err := c.Reports.Submit(ReportSubmitParams{ReportType: "fleet"})
	if err != nil {
		t.Fatalf("Submit should succeed after one retry: %v", err)
	}
	if rep.ReportID != "r-1" {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if count() != 2 {
		t.Fatalf("requests = %d, want 2", count())
	}
	assertWaits(t, waits.all(), 500*time.Millisecond)
}

func TestRetry_POSTBodyResentOnStatusRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, string(raw))
		n := len(bodies)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `{}`)
			return
		}
		_, _ = io.WriteString(w, dataEnvelope(`{"report_id":"r-1"}`))
	}))
	t.Cleanup(srv.Close)

	c, _ := retryClient(t, srv.URL)
	if _, err := c.Reports.Submit(ReportSubmitParams{ReportType: "fleet"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("got %d requests, want 2", len(bodies))
	}
	if bodies[0] == "" || bodies[0] != bodies[1] {
		t.Fatalf("retry must resend the same body, got %q then %q", bodies[0], bodies[1])
	}
}

// --- Backoff arithmetic ---

func TestBackoffDelay(t *testing.T) {
	const ceiling = 60 * time.Second
	cases := []struct {
		name    string
		factor  time.Duration
		attempt int
		ceiling time.Duration
		want    time.Duration
	}{
		{"first retry", 500 * time.Millisecond, 0, ceiling, 500 * time.Millisecond},
		{"second retry", 500 * time.Millisecond, 1, ceiling, time.Second},
		{"third retry", 500 * time.Millisecond, 2, ceiling, 2 * time.Second},
		{"capped at ceiling", 500 * time.Millisecond, 20, ceiling, ceiling},
		{"zero factor", 0, 3, ceiling, 0},
		{"zero ceiling", time.Second, 3, 0, 0},
		{"negative attempt treated as first", time.Second, -1, ceiling, time.Second},
		{"huge factor stays at ceiling", math.MaxInt64 / 2, 5, ceiling, ceiling},
		{"huge factor and huge attempt stay at ceiling", math.MaxInt64 / 2, 1000, ceiling, ceiling},
		{"exponent is clamped at 2^32", time.Nanosecond, 1000, ceiling, time.Nanosecond << maxBackoffExponent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := backoffDelay(tc.factor, tc.attempt, tc.ceiling)
			if got != tc.want {
				t.Fatalf("backoffDelay(%v, %d, %v) = %v, want %v", tc.factor, tc.attempt, tc.ceiling, got, tc.want)
			}
			if got < 0 {
				t.Fatalf("backoffDelay returned a negative duration: %v", got)
			}
		})
	}
}

func TestRetry_BackoffOverflowStaysCapped(t *testing.T) {
	srv, count := scriptedServer(t, scriptStep{status: 429, body: `{}`})
	c, waits := retryClient(t, srv.URL,
		WithMaxRetries(20),
		WithBackoffFactor(math.MaxInt64/2),
	)

	if _, err := c.Stat(); err == nil {
		t.Fatal("expected rate limit error")
	}
	if count() != 21 {
		t.Fatalf("requests = %d, want 21", count())
	}
	got := waits.all()
	if len(got) != 20 {
		t.Fatalf("waits = %d, want 20", len(got))
	}
	for i, d := range got {
		if d != 60*time.Second {
			t.Fatalf("wait %d = %v, want the 60s ceiling", i, d)
		}
	}
}

func TestRetry_HighAttemptCountNeverGoesNegative(t *testing.T) {
	srv, _ := scriptedServer(t, scriptStep{status: 429, body: `{}`})
	c, waits := retryClient(t, srv.URL, WithMaxRetries(35), WithBackoffFactor(time.Nanosecond))

	if _, err := c.Stat(); err == nil {
		t.Fatal("expected rate limit error")
	}
	for i, d := range waits.all() {
		if d < 0 || d > 60*time.Second {
			t.Fatalf("wait %d = %v, outside [0, 60s]", i, d)
		}
	}
}

// --- Retry-After parsing ---

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   time.Duration
		ok     bool
	}{
		{"empty", "", 0, false},
		{"whitespace", "   ", 0, false},
		{"integer", "2", 2 * time.Second, true},
		{"decimal", "1.5", 1500 * time.Millisecond, true},
		{"zero", "0", 0, true},
		{"negative", "-1", 0, false},
		{"NaN", "NaN", 0, false},
		{"infinity", "Inf", 0, false},
		{"garbage", "later", 0, false},
		{"overflowing seconds", "1e30", time.Duration(math.MaxInt64), true},
		{"future date", fixedNow.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second, true},
		{"past date", fixedNow.Add(-90 * time.Second).Format(http.TimeFormat), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tc.header, fixedNow)
			if ok != tc.ok {
				t.Fatalf("parseRetryAfter(%q) ok = %v, want %v", tc.header, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("parseRetryAfter(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

func TestClampDuration(t *testing.T) {
	cases := []struct {
		in, ceiling, want time.Duration
	}{
		{-time.Second, time.Minute, 0},
		{time.Second, time.Minute, time.Second},
		{2 * time.Minute, time.Minute, time.Minute},
		{time.Second, 0, 0},
	}
	for _, tc := range cases {
		if got := clampDuration(tc.in, tc.ceiling); got != tc.want {
			t.Fatalf("clampDuration(%v, %v) = %v, want %v", tc.in, tc.ceiling, got, tc.want)
		}
	}
}

// --- Status and error classification ---

func TestIsRetryableStatus(t *testing.T) {
	retryable := []int{408, 429, 500, 502, 503, 504, 599}
	for _, code := range retryable {
		if !IsRetryableStatus(code) {
			t.Fatalf("IsRetryableStatus(%d) = false, want true", code)
		}
	}
	notRetryable := []int{0, 100, 200, 201, 301, 400, 401, 402, 403, 404, 409, 418, 499, 600, 700}
	for _, code := range notRetryable {
		if IsRetryableStatus(code) {
			t.Fatalf("IsRetryableStatus(%d) = true, want false", code)
		}
	}
}

// timeoutError is a net.Error that reports a timeout.
type timeoutError struct{}

func (timeoutError) Error() string   { return "simulated timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestIsRetryableNetworkError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"request construction", &nonRetryableError{errors.New("bad url")}, false},
		{"context canceled", &url.Error{Op: "Get", URL: "x", Err: context.Canceled}, false},
		{"bare context deadline", context.DeadlineExceeded, false},
		{"wrapped caller context deadline", &url.Error{Op: "Get", URL: "x", Err: context.DeadlineExceeded}, false},
		{"client timeout", &url.Error{Op: "Get", URL: "x", Err: timeoutError{}}, true},
		{"connection refused", &url.Error{Op: "Get", URL: "x", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}, true},
		{"unknown certificate authority", &url.Error{Op: "Get", URL: "x", Err: x509.UnknownAuthorityError{}}, false},
		{"invalid certificate", &url.Error{Op: "Get", URL: "x", Err: x509.CertificateInvalidError{Reason: x509.Expired}}, false},
		{"hostname mismatch", &url.Error{Op: "Get", URL: "x", Err: x509.HostnameError{Certificate: &x509.Certificate{}, Host: "example.test"}}, false},
		{"invalid certificate by pointer", &url.Error{Op: "Get", URL: "x", Err: &x509.CertificateInvalidError{Reason: x509.Expired}}, false},
		{"unexpected eof", io.ErrUnexpectedEOF, true},
		{"eof", io.EOF, true},
		{"non-retryable wrapping eof", &nonRetryableError{io.EOF}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableNetworkError(tc.err); got != tc.want {
				t.Fatalf("isRetryableNetworkError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRetry_ClientTimeoutIsRetriedOnGet(t *testing.T) {
	// The handler blocks until the test ends; the client timeout is what fails
	// each attempt, and a timeout is a retryable transport error on GET.
	block := make(chan struct{})
	var mu sync.Mutex
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		<-block
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	c, waits := retryClient(t, srv.URL, WithTimeout(20*time.Millisecond), WithMaxRetries(1))
	if _, err := c.Stat(); err == nil {
		t.Fatal("expected timeout error")
	}
	mu.Lock()
	got := count
	mu.Unlock()
	if got != 2 {
		t.Fatalf("requests = %d, want 2 (1 + 1 retry)", got)
	}
	assertWaits(t, waits.all(), 500*time.Millisecond)
}

func TestNonRetryableError_WrapsCause(t *testing.T) {
	cause := errors.New("cause")
	err := &nonRetryableError{cause}
	if err.Error() != "cause" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "cause")
	}
	if !errors.Is(err, cause) {
		t.Fatal("nonRetryableError must unwrap to its cause")
	}
}

func TestRetry_RequestConstructionFailureNotRetried(t *testing.T) {
	// A control character makes the URL unparseable, so the request never
	// leaves the process and repeating it would fail identically.
	c, waits := retryClient(t, "http://invalid\x7fhost")

	_, err := c.Stat()
	if err == nil {
		t.Fatal("expected a request construction error")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "after 1 attempt(s)") {
		t.Fatalf("construction failures must not be retried, got %q", err.Error())
	}
	if got := waits.all(); len(got) != 0 {
		t.Fatalf("waits = %v, want none", got)
	}
}
