// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
)

const (
	// Number of retries performed when the server returns a 503
	// response with a 'Retry-after' header.  A request will be issued
	// at most NumberOfRetries + 1 times.
	NumberOfRetries = 4

	RetryAfterHeaderName = "Retry-After"
)

// Client represents a way to communicating with a MAAS API instance.
// It is stateless, so it can have concurrent requests in progress.
type Client struct {
	APIURL *url.URL
	Signer OAuthSigner
}

// ServerError is an http error (or at least, a non-2xx result) received from
// the server.  It contains the numerical HTTP status code as well as an error
// string and the response's headers.
type ServerError struct {
	error
	StatusCode  int
	Header      http.Header
	BodyMessage string
}

// GetServerError returns the ServerError from the cause of the error if it is a
// ServerError, and also returns the bool to indicate if it was a ServerError or
// not.
func GetServerError(err error) (ServerError, bool) {
	svrErr, ok := errors.Cause(err).(ServerError)
	return svrErr, ok
}

// readAndClose reads and closes the given ReadCloser.
//
// Trying to read from a nil simply returns nil, no error.
func readAndClose(stream io.ReadCloser) ([]byte, error) {
	if stream == nil {
		return nil, nil
	}
	defer stream.Close()
	return ioutil.ReadAll(stream)
}

// dispatchRequest sends a request to the server, and interprets the response.
// Client-side errors will return an empty response and a non-nil error.  For
// server-side errors however (i.e. responses with a non 2XX status code), the
// returned error will be ServerError and the returned body will reflect the
// server's response.  If the server returns a 503 response with a 'Retry-after'
// header, the request will be transparenty retried.
func (client Client) dispatchRequest(request *http.Request) ([]byte, error) {
	// First, store the request's body into a byte[] to be able to restore it
	// after each request.
	bodyContent, err := readAndClose(request.Body)
	if err != nil {
		return nil, err
	}
	for retry := 0; retry < NumberOfRetries; retry++ {
		// Restore body before issuing request.
		newBody := ioutil.NopCloser(bytes.NewReader(bodyContent))
		request.Body = newBody
		body, err := client.dispatchSingleRequest(request)
		// If this is a 503 response with a non-void "Retry-After" header: wait
		// as instructed and retry the request.
		if err != nil {
			serverError, ok := errors.Cause(err).(ServerError)
			if ok && serverError.StatusCode == http.StatusServiceUnavailable {
				retry_time_int, errConv := strconv.Atoi(serverError.Header.Get(RetryAfterHeaderName))
				if errConv == nil {
					select {
					case <-time.After(time.Duration(retry_time_int) * time.Second):
					}
					continue
				}
			}
		}
		return body, err
	}
	// Restore body before issuing request.
	newBody := ioutil.NopCloser(bytes.NewReader(bodyContent))
	request.Body = newBody
	return client.dispatchSingleRequest(request)
}

func (client Client) dispatchSingleRequest(request *http.Request) ([]byte, error) {
	client.Signer.OAuthSign(request)
	httpClient := http.Client{}
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	request.Close = true
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	body, err := readAndClose(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		err := errors.Errorf("ServerError: %v (%s)", response.Status, body)
		return body, errors.Trace(ServerError{error: err, StatusCode: response.StatusCode, Header: response.Header, BodyMessage: string(body)})
	}
	return body, nil
}

// GetURL returns the URL to a given resource on the API, based on its URI.
// The resource URI may be absolute or relative; either way the result is a
// full absolute URL including the network part.
func (client Client) GetURL(uri *url.URL) *url.URL {
	return client.APIURL.ResolveReference(uri)
}

// Get performs an HTTP "GET" to the API.  This may be either an API method
// invocation (if you pass its name in "operation") or plain resource
// retrieval (if you leave "operation" blank).
func (client Client) Get(uri *url.URL, operation string, parameters url.Values) ([]byte, error) {
	if parameters == nil {
		parameters = make(url.Values)
	}
	opParameter := parameters.Get("op")
	if opParameter != "" {
		msg := errors.Errorf("reserved parameter 'op' passed (with value '%s')", opParameter)
		return nil, msg
	}
	if operation != "" {
		parameters.Set("op", operation)
	}
	queryUrl := client.GetURL(uri)
	queryUrl.RawQuery = parameters.Encode()
	request, err := http.NewRequest("GET", queryUrl.String(), nil)
	if err != nil {
		return nil, err
	}
	return client.dispatchRequest(request)
}

// writeMultiPartFiles writes the given files as parts of a multipart message
// using the given writer.
func writeMultiPartFiles(writer *multipart.Writer, files map[string][]byte) error {
	for fileName, fileContent := range files {

		fw, err := writer.CreateFormFile(fileName, fileName)
		if err != nil {
			return err
		}
		io.Copy(fw, bytes.NewBuffer(fileContent))
	}
	return nil
}

// writeMultiPartParams writes the given parameters as parts of a multipart
// message using the given writer.
func writeMultiPartParams(writer *multipart.Writer, parameters url.Values) error {
	for key, values := range parameters {
		for _, value := range values {
			fw, err := writer.CreateFormField(key)
			if err != nil {
				return err
			}
			buffer := bytes.NewBufferString(value)
			io.Copy(fw, buffer)
		}
	}
	return nil

}

// nonIdempotentRequestFiles implements the common functionality of PUT and
// POST requests (but not GET or DELETE requests) when uploading files is
// needed.
func (client Client) nonIdempotentRequestFiles(method string, uri *url.URL, parameters url.Values, files map[string][]byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	err := writeMultiPartFiles(writer, files)
	if err != nil {
		return nil, err
	}
	err = writeMultiPartParams(writer, parameters)
	if err != nil {
		return nil, err
	}
	writer.Close()
	url := client.GetURL(uri)
	request, err := http.NewRequest(method, url.String(), buf)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return client.dispatchRequest(request)

}

// nonIdempotentRequest implements the common functionality of PUT and POST
// requests (but not GET or DELETE requests).
func (client Client) nonIdempotentRequest(method string, uri *url.URL, parameters url.Values) ([]byte, error) {
	url := client.GetURL(uri)
	request, err := http.NewRequest(method, url.String(), strings.NewReader(string(parameters.Encode())))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return client.dispatchRequest(request)
}

// Post performs an HTTP "POST" to the API.  This may be either an API method
// invocation (if you pass its name in "operation") or plain resource
// retrieval (if you leave "operation" blank).
func (client Client) Post(uri *url.URL, operation string, parameters url.Values, files map[string][]byte) ([]byte, error) {
	queryParams := url.Values{"op": {operation}}
	uri.RawQuery = queryParams.Encode()
	if files != nil {
		return client.nonIdempotentRequestFiles("POST", uri, parameters, files)
	}
	return client.nonIdempotentRequest("POST", uri, parameters)
}

// Put updates an object on the API, using an HTTP "PUT" request.
func (client Client) Put(uri *url.URL, parameters url.Values) ([]byte, error) {
	return client.nonIdempotentRequest("PUT", uri, parameters)
}

// Delete deletes an object on the API, using an HTTP "DELETE" request.
func (client Client) Delete(uri *url.URL) error {
	url := client.GetURL(uri)
	request, err := http.NewRequest("DELETE", url.String(), strings.NewReader(""))
	if err != nil {
		return err
	}
	_, err = client.dispatchRequest(request)
	if err != nil {
		return err
	}
	return nil
}

// Anonymous "signature method" implementation.
type anonSigner struct{}

func (signer anonSigner) OAuthSign(request *http.Request) error {
	return nil
}

// *anonSigner implements the OAuthSigner interface.
var _ OAuthSigner = anonSigner{}

// AddAPIVersionToURL will add the version/<version>/ suffix to the
// given URL, handling trailing slashes. It shouldn't be called with a
// URL that already includes a version.
func AddAPIVersionToURL(BaseURL, apiVersion string) string {
	baseurl := EnsureTrailingSlash(BaseURL)
	return fmt.Sprintf("%sapi/%s/", baseurl, apiVersion)
}

var apiVersionPattern = regexp.MustCompile(`^(?P<base>.*/)api/(?P<version>\d+\.\d+)/?$`)

// SplitVersionedURL splits a versioned API URL (like
// http://maas.server/MAAS/api/2.0/) into a base URL
// (http://maas.server/MAAS/) and API version (2.0). If the URL
// doesn't include a version component the bool return value will be
// false.
func SplitVersionedURL(url string) (string, string, bool) {
	if !apiVersionPattern.MatchString(url) {
		return url, "", false
	}
	version := apiVersionPattern.ReplaceAllString(url, "$version")
	baseURL := apiVersionPattern.ReplaceAllString(url, "$base")
	return baseURL, version, true
}

// NewAnonymousClient creates a client that issues anonymous requests.
// BaseURL should refer to the root of the MAAS server path, e.g.
// http://my.maas.server.example.com/MAAS/
// apiVersion should contain the version of the MAAS API that you want to use.
func NewAnonymousClient(BaseURL string, apiVersion string) (*Client, error) {
	versionedURL := AddAPIVersionToURL(BaseURL, apiVersion)
	parsedURL, err := url.Parse(versionedURL)
	if err != nil {
		return nil, err
	}
	return &Client{Signer: &anonSigner{}, APIURL: parsedURL}, nil
}

// NewAuthenticatedClient parses the given MAAS API key into the
// individual OAuth tokens and creates an Client that will use these
// tokens to sign the requests it issues.
// versionedURL should be the location of the versioned API root of
// the MAAS server, e.g.:
// http://my.maas.server.example.com/MAAS/api/2.0/
func NewAuthenticatedClient(versionedURL, apiKey string) (*Client, error) {
	elements := strings.Split(apiKey, ":")
	if len(elements) != 3 {
		errString := fmt.Sprintf("invalid API key %q; expected \"<consumer secret>:<token key>:<token secret>\"", apiKey)
		return nil, errors.NewNotValid(nil, errString)
	}
	token := &OAuthToken{
		ConsumerKey: elements[0],
		// The consumer secret is the empty string in MAAS' authentication.
		ConsumerSecret: "",
		TokenKey:       elements[1],
		TokenSecret:    elements[2],
	}
	signer, err := NewPlainTestOAuthSigner(token, "MAAS API")
	if err != nil {
		return nil, err
	}
	parsedURL, err := url.Parse(EnsureTrailingSlash(versionedURL))
	if err != nil {
		return nil, err
	}
	return &Client{Signer: signer, APIURL: parsedURL}, nil
}
