// An HTTP Client which sends json and binary requests, handling data marshalling and response processing.

package http

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"gopkg.in/goose.v1"
	"gopkg.in/goose.v1/errors"
)

const (
	contentTypeJSON        = "application/json"
	contentTypeOctetStream = "application/octet-stream"
)

func init() {
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	roundTripper := http.DefaultClient.Transport
	if transport, ok := roundTripper.(*http.Transport); ok {
		transport.DisableKeepAlives = true
	}
	http.DefaultTransport.(*http.Transport).DisableKeepAlives = true
}

type Client struct {
	http.Client
	maxSendAttempts int
}

type ErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	Title   string `json:"title"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("Failed: %d %s: %s", e.Code, e.Title, e.Message)
}

func unmarshallError(jsonBytes []byte) (*ErrorResponse, error) {
	var response ErrorResponse
	var transientObject = make(map[string]*json.RawMessage)
	if err := json.Unmarshal(jsonBytes, &transientObject); err != nil {
		return nil, err
	}
	for key, value := range transientObject {
		if err := json.Unmarshal(*value, &response); err != nil {
			return nil, err
		}
		response.Title = key
		break
	}
	if response.Code != 0 && response.Message != "" {
		return &response, nil
	}
	return nil, fmt.Errorf("Unparsable json error body: %q", jsonBytes)
}

type RequestData struct {
	ReqHeaders     http.Header
	Params         *url.Values
	ExpectedStatus []int
	ReqValue       interface{}
	RespValue      interface{}
	ReqReader      io.Reader
	ReqLength      int
	RespReader     io.ReadCloser
	RespHeaders    http.Header
}

const (
	// The maximum number of times to try sending a request before we give up
	// (assuming any unsuccessful attempts can be sensibly tried again).
	MaxSendAttempts = 3
)

var insecureClient *http.Client
var insecureClientMutex sync.Mutex

// New returns a new goose http *Client using the default net/http client.
func New() *Client {
	return &Client{*http.DefaultClient, MaxSendAttempts}
}

func NewNonSSLValidating() *Client {
	insecureClientMutex.Lock()
	httpClient := insecureClient
	if httpClient == nil {
		insecureConfig := &tls.Config{InsecureSkipVerify: true}
		insecureTransport := &http.Transport{TLSClientConfig: insecureConfig}
		insecureClient = &http.Client{Transport: insecureTransport}
		httpClient = insecureClient
	}
	insecureClientMutex.Unlock()
	return &Client{*httpClient, MaxSendAttempts}
}

func gooseAgent() string {
	return fmt.Sprintf("goose (%s)", goose.Version)
}

func createHeaders(extraHeaders http.Header, contentType, authToken string) http.Header {
	headers := make(http.Header)
	if extraHeaders != nil {
		for header, values := range extraHeaders {
			for _, value := range values {
				headers.Add(header, value)
			}
		}
	}
	if authToken != "" {
		headers.Set("X-Auth-Token", authToken)
	}
	headers.Add("Content-Type", contentType)
	headers.Add("Accept", contentType)
	headers.Add("User-Agent", gooseAgent())
	return headers
}

// JsonRequest JSON encodes and sends the object in reqData.ReqValue (if any) to the specified URL.
// Optional method arguments are passed using the RequestData object.
// Relevant RequestData fields:
// ReqHeaders: additional HTTP header values to add to the request.
// ExpectedStatus: the allowed HTTP response status values, else an error is returned.
// ReqValue: the data object to send.
// RespValue: the data object to decode the result into.
func (c *Client) JsonRequest(method, url, token string, reqData *RequestData, logger *log.Logger) (err error) {
	err = nil
	var body []byte
	if reqData.Params != nil {
		url += "?" + reqData.Params.Encode()
	}
	if reqData.ReqValue != nil {
		body, err = json.Marshal(reqData.ReqValue)
		if err != nil {
			err = errors.Newf(err, "failed marshalling the request body")
			return
		}
	}
	headers := createHeaders(reqData.ReqHeaders, contentTypeJSON, token)
	resp, err := c.sendRequest(
		method, url, bytes.NewReader(body), len(body), headers, reqData.ExpectedStatus, logger)
	if err != nil {
		return
	}
	reqData.RespHeaders = resp.Header
	defer resp.Body.Close()
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Newf(err, "failed reading the response body")
		return
	}

	if len(respData) > 0 {
		if reqData.RespValue != nil {
			err = json.Unmarshal(respData, &reqData.RespValue)
			if err != nil {
				err = errors.Newf(err, "failed unmarshaling the response body: %s", respData)
			}
		}
	}
	return
}

// Sends the byte array in reqData.ReqValue (if any) to the specified URL.
// Optional method arguments are passed using the RequestData object.
// Relevant RequestData fields:
// ReqHeaders: additional HTTP header values to add to the request.
// ExpectedStatus: the allowed HTTP response status values, else an error is returned.
// ReqReader: an io.Reader providing the bytes to send.
// RespReader: assigned an io.ReadCloser instance used to read the returned data..
func (c *Client) BinaryRequest(method, url, token string, reqData *RequestData, logger *log.Logger) (err error) {
	err = nil

	if reqData.Params != nil {
		url += "?" + reqData.Params.Encode()
	}
	headers := createHeaders(reqData.ReqHeaders, contentTypeOctetStream, token)
	resp, err := c.sendRequest(
		method, url, reqData.ReqReader, reqData.ReqLength, headers, reqData.ExpectedStatus, logger)
	if err != nil {
		return
	}
	reqData.RespHeaders = resp.Header
	if reqData.RespReader != nil {
		reqData.RespReader = resp.Body
	} else {
		resp.Body.Close()
	}
	return
}

// Sends the specified request to URL and checks that the HTTP response status is as expected.
// reqReader: a reader returning the data to send.
// length: the number of bytes to send.
// headers: HTTP headers to include with the request.
// expectedStatus: a slice of allowed response status codes.
func (c *Client) sendRequest(method, URL string, reqReader io.Reader, length int, headers http.Header,
	expectedStatus []int, logger *log.Logger) (*http.Response, error) {
	reqData := make([]byte, length)
	if reqReader != nil {
		nrRead, err := io.ReadFull(reqReader, reqData)
		if err != nil {
			err = errors.Newf(err, "failed reading the request data, read %v of %v bytes", nrRead, length)
			return nil, err
		}
	}
	rawResp, err := c.sendRateLimitedRequest(method, URL, headers, reqData, logger)
	if err != nil {
		return nil, err
	}
	foundStatus := false
	if len(expectedStatus) == 0 {
		expectedStatus = []int{http.StatusOK}
	}
	for _, status := range expectedStatus {
		if rawResp.StatusCode == status {
			foundStatus = true
			break
		}
	}
	if !foundStatus && len(expectedStatus) > 0 {
		err = handleError(URL, rawResp)
		rawResp.Body.Close()
		return nil, err
	}
	return rawResp, err
}

func (c *Client) sendRateLimitedRequest(method, URL string, headers http.Header, reqData []byte,
	logger *log.Logger) (resp *http.Response, err error) {
	for i := 0; i < c.maxSendAttempts; i++ {
		var reqReader io.Reader
		if reqData != nil {
			reqReader = bytes.NewReader(reqData)
		}
		req, err := http.NewRequest(method, URL, reqReader)
		if err != nil {
			err = errors.Newf(err, "failed creating the request %s", URL)
			return nil, err
		}
		for header, values := range headers {
			for _, value := range values {
				req.Header.Add(header, value)
			}
		}
		req.ContentLength = int64(len(reqData))
		resp, err = c.Do(req)
		if err != nil {
			return nil, errors.Newf(err, "failed executing the request %s", URL)
		}
		if resp.StatusCode != http.StatusRequestEntityTooLarge || resp.Header.Get("Retry-After") == "" {
			return resp, nil
		}
		resp.Body.Close()
		retryAfter, err := strconv.ParseFloat(resp.Header.Get("Retry-After"), 32)
		if err != nil {
			return nil, errors.Newf(err, "Invalid Retry-After header %s", URL)
		}
		if retryAfter == 0 {
			return nil, errors.Newf(err, "Resource limit exeeded at URL %s", URL)
		}
		if logger != nil {
			logger.Printf("Too many requests, retrying in %dms.", int(retryAfter*1000))
		}
		time.Sleep(time.Duration(retryAfter) * time.Second)
	}
	return nil, errors.Newf(err, "Maximum number of attempts (%d) reached sending request to %s", c.maxSendAttempts, URL)
}

type HttpError struct {
	StatusCode      int
	Data            map[string][]string
	url             string
	responseMessage string
}

func (e *HttpError) Error() string {
	return fmt.Sprintf("request (%s) returned unexpected status: %d; error info: %v",
		e.url,
		e.StatusCode,
		e.responseMessage,
	)
}

// The HTTP response status code was not one of those expected, so we construct an error.
// NotFound (404) codes have their own NotFound error type.
// We also make a guess at duplicate value errors.
func handleError(URL string, resp *http.Response) error {
	errBytes, _ := ioutil.ReadAll(resp.Body)
	errInfo := string(errBytes)
	// Check if we have a JSON representation of the failure, if so decode it.
	if resp.Header.Get("Content-Type") == contentTypeJSON {
		errorResponse, err := unmarshallError(errBytes)
		//TODO (hduran-8): Obtain a logger and log the error
		if err == nil {
			errInfo = errorResponse.Error()
		}
	}
	httpError := &HttpError{
		resp.StatusCode, map[string][]string(resp.Header), URL, errInfo,
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return errors.NewNotFoundf(httpError, "", "Resource at %s not found", URL)
	case http.StatusForbidden, http.StatusUnauthorized:
		return errors.NewUnauthorisedf(httpError, "", "Unauthorised URL %s", URL)
	case http.StatusBadRequest:
		dupExp, _ := regexp.Compile(".*already exists.*")
		if dupExp.Match(errBytes) {
			return errors.NewDuplicateValuef(httpError, "", string(errBytes))
		}
	}
	return httpError
}
