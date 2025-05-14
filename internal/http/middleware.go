// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"golang.org/x/net/http/httpproxy"

	"github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
)

// FileProtocolMiddleware registers support for file:// URLs on the given transport.
func FileProtocolMiddleware(transport *http.Transport) *http.Transport {
	transport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	return transport
}

// DialBreaker replicates a highly specialized CircuitBreaker pattern, which
// takes into account the current address.
type DialBreaker interface {
	// Allowed checks to see if a given address is allowed.
	Allowed(string) bool
	// Trip will cause the DialBreaker to change the breaker state
	Trip()
}

func isLocalAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

// DialContextMiddleware patches the default HTTP transport so
// that it fails when an attempt is made to dial a non-local
// host.
func DialContextMiddleware(breaker DialBreaker) TransportMiddleware {
	return func(transport *http.Transport) *http.Transport {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if !breaker.Allowed(addr) {
				return nil, errors.Errorf("access to address %q not allowed", addr)
			}

			return dialer.DialContext(ctx, network, addr)
		}
		return transport
	}
}

// LocalDialBreaker defines a DialBreaker that when tripped only allows local
// dials, anything else is prevented.
type LocalDialBreaker struct {
	allowOutgoingAccess bool
}

// NewLocalDialBreaker creates a new LocalDialBreaker with a default value.
func NewLocalDialBreaker(allowOutgoingAccess bool) *LocalDialBreaker {
	return &LocalDialBreaker{
		allowOutgoingAccess: allowOutgoingAccess,
	}
}

// Allowed checks to see if a dial is allowed to happen, or returns an error
// stating why.
func (b *LocalDialBreaker) Allowed(addr string) bool {
	if b.allowOutgoingAccess {
		return true
	}
	// If we're not allowing outgoing access, then only local addresses are
	// allowed to be dialed. Check for local only addresses.
	return isLocalAddr(addr)
}

// Trip inverts the local state of the DialBreaker.
func (b *LocalDialBreaker) Trip() {
	b.allowOutgoingAccess = !b.allowOutgoingAccess
}

// ProxyMiddleware adds a Proxy to the given transport. This implementation
// uses the http.ProxyFromEnvironment.
func ProxyMiddleware(transport *http.Transport) *http.Transport {
	transport.Proxy = getProxy
	return transport
}

var midLogger = internallogger.GetLogger("juju.http.middleware", "http")

func getProxy(req *http.Request) (*url.URL, error) {
	// Get proxy config new for each client.  Go will cache the proxy
	// settings for a process, this is a problem for long running programs.
	// And caused changes in proxy settings via model-config not to
	// be used.
	cfg := httpproxy.FromEnvironment()
	midLogger.Tracef(req.Context(), "proxy config http(%s), https(%s), no-proxy(%s)",
		cfg.HTTPProxy, cfg.HTTPSProxy, cfg.NoProxy)
	return cfg.ProxyFunc()(req.URL)
}

// ForceAttemptHTTP2Middleware forces a HTTP/2 connection if a non-zero
// Dial, DialTLS, or DialContext func or TLSClientConfig is provided to the
// Transport. Using any of these will render HTTP/2 disabled, so force the
// client to use it for requests.
func ForceAttemptHTTP2Middleware(transport *http.Transport) *http.Transport {
	transport.ForceAttemptHTTP2 = true
	return transport
}

// RequestRecorder is implemented by types that can record information about
// successful and unsuccessful http requests.
type RequestRecorder interface {
	// Record an outgoing request which produced an http.Response.
	Record(method string, url *url.URL, res *http.Response, rtt time.Duration)

	// Record an outgoing request which returned back an error.
	RecordError(method string, url *url.URL, err error)
}

// RoundTripper allows us to generate mocks for the http.RoundTripper because
// we're already in a http package.
type RoundTripper = http.RoundTripper

type roundTripRecorder struct {
	requestRecorder     RequestRecorder
	wrappedRoundTripper http.RoundTripper
}

// RoundTrip implements http.RoundTripper. If delegates the request to the
// wrapped RoundTripper and invokes the appropriate RequestRecorder methods
// depending on the outcome.
func (lr roundTripRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	res, err := lr.wrappedRoundTripper.RoundTrip(req)
	rtt := time.Since(start)

	if err != nil {
		lr.requestRecorder.RecordError(req.Method, req.URL, err)
	} else {
		lr.requestRecorder.Record(req.Method, req.URL, res, rtt)
	}

	return res, err
}

// RetryMiddleware allows retrying of certain retryable http errors.
// This only handles very specific status codes, ones that are deemed retryable:
//
//   - 502 Bad Gateway
//   - 503 Service Unavailable
//   - 504 Gateway Timeout
type retryMiddleware struct {
	policy              RetryPolicy
	wrappedRoundTripper http.RoundTripper
	clock               clock.Clock
	logger              logger.Logger
}

type RetryPolicy struct {
	Delay    time.Duration
	MaxDelay time.Duration
	Attempts int
}

// Validate validates the RetryPolicy for any issues.
func (p RetryPolicy) Validate() error {
	if p.Attempts < 1 {
		return errors.Errorf("expected at least one attempt")
	}
	if p.MaxDelay < 1 {
		return errors.Errorf("expected max delay to be a valid time")
	}
	return nil
}

// makeRetryMiddleware creates a retry transport.
func makeRetryMiddleware(transport http.RoundTripper, policy RetryPolicy, clock clock.Clock, logger logger.Logger) http.RoundTripper {
	return retryMiddleware{
		policy:              policy,
		wrappedRoundTripper: transport,
		clock:               clock,
		logger:              logger,
	}
}

type retryableErr struct{}

func (retryableErr) Error() string {
	return "retryable error"
}

// RoundTrip defines a strategy for handling retries based on the status code.
func (m retryMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	var (
		res        *http.Response
		backOffErr error
	)
	err := retry.Call(retry.CallArgs{
		Clock: m.clock,
		Func: func() error {
			if err := req.Context().Err(); err != nil {
				return err
			}
			if backOffErr != nil {
				return backOffErr
			}

			var retryable bool
			var err error
			res, retryable, err = m.roundTrip(req)
			if err != nil {
				return err
			}
			if retryable {
				return retryableErr{}
			}
			return nil
		},
		IsFatalError: func(err error) bool {
			// Work out if it's not a retryable error.
			_, ok := errors.Cause(err).(retryableErr)
			return !ok
		},
		Attempts: m.policy.Attempts,
		Delay:    m.policy.Delay,
		BackoffFunc: func(delay time.Duration, attempts int) time.Duration {
			var duration time.Duration
			duration, backOffErr = m.defaultBackoff(res, delay)
			return duration
		},
	})

	return res, err
}

func (m retryMiddleware) roundTrip(req *http.Request) (*http.Response, bool, error) {
	res, err := m.wrappedRoundTripper.RoundTrip(req)
	if err != nil {
		return nil, false, err
	}

	switch res.StatusCode {
	case http.StatusBadGateway, http.StatusGatewayTimeout:
		// The request should be retryable.
		fallthrough
	case http.StatusServiceUnavailable, http.StatusTooManyRequests:
		// The request should be retryable, but additionally should contain
		// a potential Retry-After header.
		return res, true, nil
	default:
		// Don't handle any of the following status codes.
		return res, false, nil
	}
}

// defaultBackoff attempts to workout a good backoff strategy based on the
// backoff policy or the status code from the response.
//
// RFC7231 states that the retry-after header can look like the following:
//
//   - Retry-After: <http-date>
//   - Retry-After: <delay-seconds>
func (m retryMiddleware) defaultBackoff(resp *http.Response, backoff time.Duration) (time.Duration, error) {
	if header := resp.Header.Get("Retry-After"); header != "" {
		// Attempt to parse the header from the request.
		//
		// Check for delay in seconds first, before checking for a http-date
		seconds, err := strconv.ParseInt(header, 10, 64)
		if err == nil {
			return m.clampBackoff(time.Second * time.Duration(seconds))
		}
		// Check for http-date.
		date, err := time.Parse(time.RFC1123, header)
		if err == nil {
			return m.clampBackoff(m.clock.Now().Sub(date))
		}
		url := ""
		if resp.Request != nil {
			url = resp.Request.URL.String()
		}
		m.logger.Errorf(context.TODO(), "unable to parse Retry-After header %s from %s", header, url)
	}

	return m.clampBackoff(backoff)
}

func (m retryMiddleware) clampBackoff(duration time.Duration) (time.Duration, error) {
	if m.policy.MaxDelay > 0 && duration > m.policy.MaxDelay {
		future := m.clock.Now().Add(duration)
		return duration, errors.Errorf("API request retry is not accepting further requests until %s", future.Format(time.RFC3339))
	}
	return duration, nil
}
