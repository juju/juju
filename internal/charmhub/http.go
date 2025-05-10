// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"gopkg.in/httprequest.v1"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/charmhub/path"
	jujuhttp "github.com/juju/juju/internal/http"
)

const (
	jsonContentType = "application/json"

	userAgentKey   = "User-Agent"
	userAgentValue = version.UserAgentVersion

	// defaultRetryAttempts defines the number of attempts that a default
	// HTTPClient will retry before giving up.
	// Retries are only performed on certain status codes, nothing in the 200 to
	// 400 range and a select few from the 500 range (deemed retryable):
	//
	// - http.StatusBadGateway
	// - http.StatusGatewayTimeout
	// - http.StatusServiceUnavailable
	// - http.StatusTooManyRequests
	//
	// See: juju/http package.
	defaultRetryAttempts = 3

	// defaultRetryDelay holds the amount of time after a try, a new attempt
	// will wait before another attempt.
	defaultRetryDelay = time.Second * 10

	// defaultRetryMaxDelay holds the amount of time before a giving up on a
	// request. This values includes any server response from the header
	// Retry-After.
	defaultRetryMaxDelay = time.Minute * 10
)

// HTTPClient defines a type for making the actual request. It may be an
// *http.Client.
type HTTPClient interface {
	// Do performs the *http.Request and returns an *http.Response or an error.
	Do(*http.Request) (*http.Response, error)
}

// DefaultHTTPClient creates a new HTTPClient with the default configuration.
func DefaultHTTPClient(logger corelogger.Logger) *jujuhttp.Client {
	recorder := loggingRequestRecorder{
		logger: logger.Child("transport.request-recorder", corelogger.METRICS),
	}
	return requestHTTPClient(recorder, defaultRetryPolicy())(logger)
}

// defaultRetryPolicy returns a retry policy with sane defaults for most
// requests.
func defaultRetryPolicy() jujuhttp.RetryPolicy {
	return jujuhttp.RetryPolicy{
		Attempts: defaultRetryAttempts,
		Delay:    defaultRetryDelay,
		MaxDelay: defaultRetryMaxDelay,
	}
}

type loggingRequestRecorder struct {
	logger corelogger.Logger
}

// Record an outgoing request which produced an http.Response.
func (r loggingRequestRecorder) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {
	if r.logger.IsLevelEnabled(corelogger.TRACE) {
		r.logger.Tracef(context.TODO(), "request (method: %q, host: %q, path: %q, status: %q, duration: %s)", method, url.Host, url.Path, res.Status, rtt)
	}
}

// RecordError records an outgoing request which returned an error.
func (r loggingRequestRecorder) RecordError(method string, url *url.URL, err error) {
	if r.logger.IsLevelEnabled(corelogger.TRACE) {
		r.logger.Tracef(context.TODO(), "request error (method: %q, host: %q, path: %q, err: %s)", method, url.Host, url.Path, err)
	}
}

// requestHTTPClient returns a function that creates a new HTTPClient that
// records the requests.
func requestHTTPClient(recorder jujuhttp.RequestRecorder, policy jujuhttp.RetryPolicy) func(corelogger.Logger) *jujuhttp.Client {
	return func(logger corelogger.Logger) *jujuhttp.Client {
		return jujuhttp.NewClient(
			jujuhttp.WithRequestRecorder(recorder),
			jujuhttp.WithRequestRetrier(policy),
			jujuhttp.WithLogger(logger.Child("transport", corelogger.CHARMHUB, corelogger.HTTP)),
		)
	}
}

// apiRequester creates a wrapper around the HTTPClient to allow for better
// error handling.
type apiRequester struct {
	httpClient HTTPClient
	logger     corelogger.Logger
	retryDelay time.Duration
}

// newAPIRequester creates a new http.Client for making requests to a server.
func newAPIRequester(httpClient HTTPClient, logger corelogger.Logger) *apiRequester {
	return &apiRequester{
		httpClient: httpClient,
		logger:     logger,
		retryDelay: 3 * time.Second,
	}
}

// Do performs the *http.Request and returns a *http.Response or an error.
//
// Handle empty response (io.EOF) errors specially and retry. The reason for
// this is we get these errors from Charmhub fairly regularly (they're not
// valid HTTP responses as there are no headers; they're empty responses).
func (t *apiRequester) Do(req *http.Request) (*http.Response, error) {
	// To retry requests with a body, we need to read the entire body in
	// up-front, otherwise it'll be empty on retries.
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, errors.Annotate(err, "reading request body")
		}
		err = req.Body.Close()
		if err != nil {
			return nil, errors.Annotate(err, "closing request body")
		}
	}

	// Try a fixed number of attempts with a doubling delay in between.
	var resp *http.Response
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			if body != nil {
				req.Body = io.NopCloser(bytes.NewReader(body))
			}
			var err error
			resp, err = t.doOnce(req)
			return err
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, io.EOF)
		},
		NotifyFunc: func(lastError error, attempt int) {
			t.logger.Errorf(req.Context(), "Charmhub API error (attempt %d): %v", attempt, lastError)
		},
		Attempts: 2,
		Delay:    t.retryDelay,
		Clock:    clock.WallClock,
		Stop:     req.Context().Done(),
	})
	return resp, err
}

func (t *apiRequester) doOnce(req *http.Request) (*http.Response, error) {
	resp, err := t.httpClient.Do(req)
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
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
		return nil, errors.Errorf(`server error %q`, req.URL.String())
	}

	// We expect that we always have a valid content-type from the server, once
	// we've checked that we don't get a 5xx error. Given that we send Accept
	// header of application/json, I would only ever expect to see that.
	// Everything will be incorrectly formatted.
	if contentType := resp.Header.Get("Content-Type"); contentType != jsonContentType {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()

		if potentialInvalidURL {
			return nil, errors.Errorf(`unexpected charm-hub url %q when parsing headers`, req.URL.String())
		}
		return nil, errors.Errorf(`unexpected content-type from server %q`, contentType)
	}

	return resp, nil
}

// apiRequestLogger creates a wrapper around the HTTP client to allow for better
// logging.
type apiRequestLogger struct {
	httpClient HTTPClient
	logger     corelogger.Logger
}

// newAPIRequesterLogger creates a new HTTPClient that allows logging of requests
// for every request.
func newAPIRequesterLogger(httpClient HTTPClient, logger corelogger.Logger) *apiRequestLogger {
	return &apiRequestLogger{
		httpClient: httpClient,
		logger:     logger,
	}
}

// Do performs the request and logs the request and response if tracing is enabled.
func (t *apiRequestLogger) Do(req *http.Request) (*http.Response, error) {
	if t.logger.IsLevelEnabled(corelogger.TRACE) {
		if data, err := httputil.DumpRequest(req, true); err == nil {
			t.logger.Tracef(req.Context(), "%s request %s", req.Method, data)
		} else {
			t.logger.Tracef(req.Context(), "%s request DumpRequest error %s", req.Method, err.Error())
		}
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if t.logger.IsLevelEnabled(corelogger.TRACE) {
		if data, err := httputil.DumpResponse(resp, true); err == nil {
			t.logger.Tracef(req.Context(), "%s response %s", req.Method, data)
		} else {
			t.logger.Tracef(req.Context(), "%s response DumpResponse error %s", req.Method, err.Error())
		}
	}

	return resp, err
}

// restResponse abstracts away the underlying response from the implementation.
type restResponse struct {
	StatusCode int
}

// RESTClient defines a type for making requests to a server.
type RESTClient interface {
	// Get performs GET requests to a given Path.
	Get(context.Context, path.Path, interface{}) (restResponse, error)
	// Post performs POST requests to a given Path.
	Post(context.Context, path.Path, http.Header, interface{}, interface{}) (restResponse, error)
}

// httpRESTClient represents a RESTClient that expects to interact with an
// HTTPClient.
type httpRESTClient struct {
	httpClient HTTPClient
}

// newHTTPRESTClient creates a new httpRESTClient
func newHTTPRESTClient(httpClient HTTPClient) *httpRESTClient {
	return &httpRESTClient{
		httpClient: httpClient,
	}
}

// Get makes a GET request to the given path in the CharmHub (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *httpRESTClient) Get(ctx context.Context, path path.Path, result interface{}) (restResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", path.String(), nil)
	if err != nil {
		return restResponse{}, errors.Annotate(err, "can not make new request")
	}

	// Compose the request headers.
	req.Header = make(http.Header)
	req.Header.Set("Accept", jsonContentType)
	req.Header.Set("Content-Type", jsonContentType)
	req.Header.Set(userAgentKey, userAgentValue)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return restResponse{}, errors.Trace(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse the response.
	if err := httprequest.UnmarshalJSONResponse(resp, result); err != nil {
		return restResponse{}, errors.Annotate(err, "charm hub client get")
	}

	return restResponse{
		StatusCode: resp.StatusCode,
	}, nil
}

// Post makes a POST request to the given path in the CharmHub (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *httpRESTClient) Post(ctx context.Context, path path.Path, headers http.Header, body, result interface{}) (restResponse, error) {
	buffer := new(bytes.Buffer)
	if err := json.NewEncoder(buffer).Encode(body); err != nil {
		return restResponse{}, errors.Trace(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", path.String(), buffer)
	if err != nil {
		return restResponse{}, errors.Annotate(err, "can not make new request")
	}

	// Compose the request headers.
	req.Header = make(http.Header)
	req.Header.Set("Accept", jsonContentType)
	req.Header.Set("Content-Type", jsonContentType)
	req.Header.Set(userAgentKey, userAgentValue)

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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return restResponse{}, errors.Trace(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse the response.
	if err := httprequest.UnmarshalJSONResponse(resp, result); err != nil {
		return restResponse{}, errors.Annotate(err, "charm hub client post")
	}
	return restResponse{
		StatusCode: resp.StatusCode,
	}, nil
}
