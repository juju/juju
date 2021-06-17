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
)

// MIME represents a MIME type for identifying requests and response bodies.
type MIME = string

const (
	// JSON represents the MIME type for JSON request and response types.
	JSON MIME = "application/json"
)

// Transport defines a type for making the actual request.
type Transport interface {
	// Do performs the *http.Request and returns a *http.Response or an error
	// if it fails to construct the transport.
	Do(*http.Request) (*http.Response, error)
}

// DefaultHTTPTransport creates a new HTTPTransport.
func DefaultHTTPTransport(logger Logger) Transport {
	return RequestRecorderHTTPTransport(loggingRequestRecorder{
		logger: logger,
	})(logger)
}

type loggingRequestRecorder struct {
	logger Logger
}

// Record an outgoing request which produced an http.Response.
func (r loggingRequestRecorder) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {

}

// Record an outgoing request which returned back an error.
func (r loggingRequestRecorder) RecordError(method string, url *url.URL, err error) {

}

// RequestRecorderHTTPTransport creates a new HTTPTransport that records the
// requests.
func RequestRecorderHTTPTransport(recorder jujuhttp.RequestRecorder) func(logger Logger) Transport {
	return func(logger Logger) Transport {
		return jujuhttp.NewClient(
			jujuhttp.WithRequestRecorder(recorder),
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

	if resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusNoContent {
		return resp, nil
	}

	if data, err := httputil.DumpResponse(resp, true); err == nil {
		t.logger.Errorf("Response %s", data)
	} else {
		t.logger.Errorf("Response DumpResponse error %s", err.Error())
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
