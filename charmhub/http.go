// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"time"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/charmhub/path"
	labels "github.com/juju/juju/core/logger"
)

// MIME represents a MIME type for identifying requests and response bodies.
type MIME = string

const (
	// JSON represents the MIME type for JSON request and response types.
	JSON MIME = "application/json"
)

const (
	// DefaultRetryAttempts defines the number of attempts that a default http
	// transport will retry before giving up.
	// Retries are only performed on certain status codes, nothing in the 200 to
	// 400 range and a select few from the 500 range (deemed retryable):
	//
	// - http.StatusBadGateway
	// - http.StatusGatewayTimeout
	// - http.StatusServiceUnavailable
	// - http.StatusTooManyRequests
	//
	// See: juju/http package.
	DefaultRetryAttempts = 3

	// DefaultRetryDelay holds the amount of time after a try, a new attempt
	// will wait before another attempt.
	DefaultRetryDelay = time.Second * 10

	// DefaultRetryMaxDelay holds the amount of time before a giving up on a
	// request. This values includes any server response from the header
	// Retry-After.
	DefaultRetryMaxDelay = time.Minute * 10
)

// Transport defines a type for making the actual request.
type Transport interface {
	// Do performs the *http.Request and returns a *http.Response or an error
	// if it fails to construct the transport.
	Do(*http.Request) (*http.Response, error)
}

// DefaultHTTPTransport creates a new HTTPTransport.
func DefaultHTTPTransport(logger Logger) Transport {
	return RequestHTTPTransport(loggingRequestRecorder{
		logger: logger.Child("request-recorder", labels.HTTP),
	}, DefaultRetryPolicy())(logger)
}

// DefaultRetryPolicy returns a retry policy with sane defaults for most
// requests.
func DefaultRetryPolicy() jujuhttp.RetryPolicy {
	return jujuhttp.RetryPolicy{
		Attempts: DefaultRetryAttempts,
		Delay:    DefaultRetryDelay,
		MaxDelay: DefaultRetryMaxDelay,
	}
}

type loggingRequestRecorder struct {
	logger Logger
}

// Record an outgoing request which produced an http.Response.
func (r loggingRequestRecorder) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {
	if r.logger.IsTraceEnabled() {
		r.logger.Tracef("request (method: %q, host: %q, path: %q, status: %q, duration: %s)", method, url.Host, url.Path, res.Status, rtt)
	}
}

// Record an outgoing request which returned back an error.
func (r loggingRequestRecorder) RecordError(method string, url *url.URL, err error) {
	if r.logger.IsTraceEnabled() {
		r.logger.Tracef("request error (method: %q, host: %q, path: %q, err: %s)", method, url.Host, url.Path, err)
	}
}

// RequestHTTPTransport creates a new HTTPTransport that records the
// requests.
func RequestHTTPTransport(recorder jujuhttp.RequestRecorder, policy jujuhttp.RetryPolicy) func(logger Logger) Transport {
	return func(logger Logger) Transport {
		return jujuhttp.NewClient(
			jujuhttp.WithRequestRecorder(recorder),
			jujuhttp.WithRequestRetrier(policy),
			jujuhttp.WithLogger(logger),
		)
	}
}

// APIRequester creates a wrapper around the transport to allow for better
// error handling.
type APIRequester struct {
	transport Transport
	logger    Logger
}

// NewAPIRequester creates a new http.Client for making requests to a server.
func NewAPIRequester(transport Transport, logger Logger) *APIRequester {
	return &APIRequester{
		transport: transport,
		logger:    logger,
	}
}

// Do performs the *http.Request and returns a *http.Response or an error
// if it fails to construct the transport.
func (t *APIRequester) Do(req *http.Request) (*http.Response, error) {
	resp, err := t.transport.Do(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusNoContent {
		return resp, nil
	}

	var potentialInvalidURL bool
	if resp.StatusCode == http.StatusNotFound {
		potentialInvalidURL = true
	} else if resp.StatusCode >= http.StatusInternalServerError && resp.StatusCode <= http.StatusNetworkAuthenticationRequired {
		defer func() {
			_, _ = io.Copy(ioutil.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
		return nil, errors.Errorf(`server error %q`, req.URL.String())
	}

	// We expect that we always have a valid content-type from the server, once
	// we've checked that we don't get a 5xx error. Given that we send Accept
	// header of application/json, I would only ever expect to see that.
	// Everything will be incorrectly formatted.
	if contentType := resp.Header.Get("Content-Type"); contentType != JSON {
		defer func() {
			_, _ = io.Copy(ioutil.Discard, resp.Body)
			_ = resp.Body.Close()
		}()

		if potentialInvalidURL {
			return nil, errors.Errorf(`unexpected charm-hub url %q when parsing headers`, req.URL.String())
		}
		return nil, errors.Errorf(`unexpected content-type from server %q`, contentType)
	}

	return resp, nil
}

// APIRequestLogger creates a wrapper around the transport to allow for better
// logging.
type APIRequestLogger struct {
	transport Transport
	logger    Logger
}

// NewAPIRequesterLogger creates a new Transport that allows logging of requests
// for every request.
func NewAPIRequesterLogger(transport Transport, logger Logger) *APIRequestLogger {
	return &APIRequestLogger{
		transport: transport,
		logger:    logger,
	}
}

// Do performs the *http.Request and returns a *http.Response or an error
// if it fails to construct the transport.
func (t *APIRequestLogger) Do(req *http.Request) (*http.Response, error) {
	if t.logger.IsTraceEnabled() {
		if data, err := httputil.DumpRequest(req, true); err == nil {
			t.logger.Tracef("%s request %s", req.Method, data)
		} else {
			t.logger.Tracef("%s request DumpRequest error %s", req.Method, err.Error())
		}
	}

	resp, err := t.transport.Do(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if t.logger.IsTraceEnabled() {
		if data, err := httputil.DumpResponse(resp, true); err == nil {
			t.logger.Tracef("%s response %s", req.Method, data)
		} else {
			t.logger.Tracef("%s response DumpResponse error %s", req.Method, err.Error())
		}
	}

	return resp, err
}

// RESTResponse abstracts away the underlying response from the implementation.
type RESTResponse struct {
	StatusCode int
}

// RESTClient defines a type for making requests to a server.
type RESTClient interface {
	// Get performs GET requests to a given Path.
	Get(context.Context, path.Path, interface{}) (RESTResponse, error)
	// Post performs POST requests to a given Path.
	Post(context.Context, path.Path, http.Header, interface{}, interface{}) (RESTResponse, error)
}

// HTTPRESTClient represents a RESTClient that expects to interact with a
// HTTP transport.
type HTTPRESTClient struct {
	transport Transport
	headers   http.Header
}

// NewHTTPRESTClient creates a new HTTPRESTClient
func NewHTTPRESTClient(transport Transport, headers http.Header) *HTTPRESTClient {
	return &HTTPRESTClient{
		transport: transport,
		headers:   headers,
	}
}

// Get makes a GET request to the given path in the CharmHub (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *HTTPRESTClient) Get(ctx context.Context, path path.Path, result interface{}) (RESTResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", path.String(), nil)
	if err != nil {
		return RESTResponse{}, errors.Annotate(err, "can not make new request")
	}

	// Compose the request headers.
	headers := make(http.Header)
	headers.Set("Accept", JSON)
	headers.Set("Content-Type", JSON)

	req.Header = c.composeHeaders(headers)

	resp, err := c.transport.Do(req)
	if err != nil {
		return RESTResponse{}, errors.Trace(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse the response.
	if err := httprequest.UnmarshalJSONResponse(resp, result); err != nil {
		return RESTResponse{}, errors.Annotate(err, "charm hub client get")
	}

	return RESTResponse{
		StatusCode: resp.StatusCode,
	}, nil
}

// Post makes a POST request to the given path in the CharmHub (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *HTTPRESTClient) Post(ctx context.Context, path path.Path, headers http.Header, body, result interface{}) (RESTResponse, error) {
	buffer := new(bytes.Buffer)
	if err := json.NewEncoder(buffer).Encode(body); err != nil {
		return RESTResponse{}, errors.Trace(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", path.String(), buffer)
	if err != nil {
		return RESTResponse{}, errors.Annotate(err, "can not make new request")
	}

	// Compose the request headers.
	req.Header = make(http.Header)
	req.Header.Set("Accept", JSON)
	req.Header.Set("Content-Type", JSON)
	req.Header = c.composeHeaders(req.Header)

	// Add any headers specific to this request (in sorted order).
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range headers[k] {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.transport.Do(req)
	if err != nil {
		return RESTResponse{}, errors.Trace(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse the response.
	if err := httprequest.UnmarshalJSONResponse(resp, result); err != nil {
		return RESTResponse{}, errors.Annotate(err, "charm hub client post")
	}
	return RESTResponse{
		StatusCode: resp.StatusCode,
	}, nil
}

// composeHeaders creates a new set of headers from scratch.
func (c *HTTPRESTClient) composeHeaders(headers http.Header) http.Header {
	result := make(http.Header)
	// Consume the new headers.
	for k, vs := range headers {
		for _, v := range vs {
			result.Add(k, v)
		}
	}
	// Add the client's headers as well.
	for k, vs := range c.headers {
		for _, v := range vs {
			result.Add(k, v)
		}
	}
	return result
}
