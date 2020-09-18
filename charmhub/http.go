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

	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/charmhub/path"
)

// Transport defines a type for making the actual request.
type Transport interface {
	// Do performs the *http.Request and returns a *http.Response or an error
	// if it fails to construct the transport.
	Do(*http.Request) (*http.Response, error)
}

// DefaultHTTPTransport creates a new HTTPTransport.
func DefaultHTTPTransport() *http.Client {
	return &http.Client{}
}

// APIRequester creates a wrapper around the transport to allow for better
// error handling.
type APIRequester struct {
	transport Transport
}

// NewAPIRequester creates a new http.Client for making requests to a server.
func NewAPIRequester(transport Transport) *APIRequester {
	return &APIRequester{
		transport: transport,
	}
}

// Do performs the *http.Request and returns a *http.Response or an error
// if it fails to construct the transport.
func (t *APIRequester) Do(req *http.Request) (*http.Response, error) {
	resp, err := t.transport.Do(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusNoContent {
		return resp, nil
	}

	var potentialInvalidURL bool
	if resp.StatusCode == http.StatusNotFound {
		potentialInvalidURL = true
	}

	if contentType := resp.Header.Get("Content-Type"); contentType != "application/json" {
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

// RESTClient defines a type for making requests to a server.
type RESTClient interface {
	// Get performs GET requests to a given Path.
	Get(context.Context, path.Path, interface{}) error
	// Post performs POST requests to a given Path.
	Post(context.Context, path.Path, interface{}, interface{}) error
}

// HTTPRESTClient represents a RESTClient that expects to interact with a
// HTTP transport.
type HTTPRESTClient struct {
	transport Transport
	headers   http.Header
	logger    Logger
}

// NewHTTPRESTClient creates a new HTTPRESTClient
func NewHTTPRESTClient(transport Transport, headers http.Header, logger Logger) *HTTPRESTClient {
	return &HTTPRESTClient{
		transport: transport,
		headers:   headers,
		logger:    logger,
	}
}

// Get makes a GET request to the given path in the CharmHub (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *HTTPRESTClient) Get(ctx context.Context, path path.Path, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", path.String(), nil)
	if err != nil {
		return errors.Annotate(err, "can not make new request")
	}
	resp, err := c.transport.Do(req)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if c.logger.IsTraceEnabled() {
		if data, err := httputil.DumpResponse(resp, true); err == nil {
			c.logger.Tracef("Get response %s", data)
		} else {
			c.logger.Tracef("Get response DumpResponse error %s", err.Error())
		}
	}
	// Parse the response.
	if err := httprequest.UnmarshalJSONResponse(resp, result); err != nil {
		return errors.Annotate(err, "charm hub client get")
	}
	return nil
}

// Post makes a POST request to the given path in the CharmHub (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *HTTPRESTClient) Post(ctx context.Context, path path.Path, body, result interface{}) error {
	buffer := new(bytes.Buffer)
	if err := json.NewEncoder(buffer).Encode(body); err != nil {
		return errors.Trace(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", path.String(), buffer)
	if err != nil {
		return errors.Annotate(err, "can not make new request")
	}

	// Compose the request headers.
	headers := make(http.Header)
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")

	req.Header = c.composeHeaders(headers)

	resp, err := c.transport.Do(req)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse the response.
	if err := httprequest.UnmarshalJSONResponse(resp, result); err != nil {
		return errors.Annotate(err, "charm hub client get")
	}
	return nil
}

// composeHeaders creates a new set of headers from scratch.
func (c *HTTPRESTClient) composeHeaders(headers http.Header) http.Header {
	result := make(http.Header)
	// Consume the new headers.
	for k := range headers {
		result.Set(k, headers.Get(k))
	}
	// Ensure the client headers overwrite the existing headers.
	for k := range c.headers {
		result.Set(k, c.headers.Get(k))
	}
	return result
}
