package store_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/url"
	"os"
	"time"
)

type HTTPSuite struct{}

var testServer = NewTestHTTPServer("http://localhost:4444", 5*time.Second)

func (s *HTTPSuite) SetUpSuite(c *C) {
	testServer.Start()
}

func (s *HTTPSuite) TearDownTest(c *C) {
	testServer.Flush()
}

type TestHTTPServer struct {
	URL      string
	Timeout  time.Duration
	started  bool
	request  chan *http.Request
	response chan ResponseFunc
	pending  chan bool
}

func NewTestHTTPServer(url_ string, timeout time.Duration) *TestHTTPServer {
	return &TestHTTPServer{URL: url_, Timeout: timeout}
}

type Response struct {
	Status  int
	Headers map[string]string
	Body    string
}

type ResponseFunc func(path string) Response

func (s *TestHTTPServer) Start() {
	if s.started {
		return
	}
	s.started = true

	s.request = make(chan *http.Request, 64)
	s.response = make(chan ResponseFunc, 64)
	s.pending = make(chan bool, 64)

	url_, _ := url.Parse(s.URL)
	go http.ListenAndServe(url_.Host, s)

	s.Response(203, nil, "")
	for {
		// Wait for it to be up.
		resp, err := http.Get(s.URL)
		if err == nil && resp.StatusCode == 203 {
			break
		}
		time.Sleep(1e8)
	}
	s.WaitRequest() // Consume dummy request.
}

// FlushRequests discards requests which were not yet consumed by WaitRequest.
func (s *TestHTTPServer) Flush() {
	for {
		select {
		case <-s.request:
		case <-s.response:
		default:
			return
		}
	}
}

func body(req *http.Request) string {
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func (s *TestHTTPServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.ParseMultipartForm(1e6)
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(data))
	s.request <- req
	var resp Response
	select {
	case respFunc := <-s.response:
		resp = respFunc(req.URL.Path)
	case <-time.After(s.Timeout):
		const msg = "ERROR: Timeout waiting for test to prepare a response\n"
		fmt.Fprintf(os.Stderr, msg)
		resp = Response{500, nil, msg}
	}
	if resp.Headers != nil {
		h := w.Header()
		for k, v := range resp.Headers {
			h.Set(k, v)
		}
	}
	if resp.Status != 0 {
		w.WriteHeader(resp.Status)
	}
	w.Write([]byte(resp.Body))
}

// WaitRequests returns the next n requests made to the http server from
// the queue. If not enough requests were previously made, it waits until
// the timeout value for them to be made.
func (s *TestHTTPServer) WaitRequests(n int) []*http.Request {
	reqs := make([]*http.Request, 0, n)
	for i := 0; i < n; i++ {
		select {
		case req := <-s.request:
			reqs = append(reqs, req)
		case <-time.After(s.Timeout):
			panic("Timeout waiting for request")
		}
	}
	return reqs
}

// WaitRequest returns the next request made to the http server from
// the queue. If no requests were previously made, it waits until the
// timeout value for one to be made.
func (s *TestHTTPServer) WaitRequest() *http.Request {
	return s.WaitRequests(1)[0]
}

// ResponseFunc prepares the test server to respond the following n
// requests using f to build each response.
func (s *TestHTTPServer) ResponseFunc(n int, f ResponseFunc) {
	for i := 0; i < n; i++ {
		s.response <- f
	}
}

// ResponseMap maps request paths to responses.
type ResponseMap map[string]Response

// ResponseMap prepares the test server to respond the following n
// requests using the m to obtain the responses.
func (s *TestHTTPServer) ResponseMap(n int, m ResponseMap) {
	f := func(path string) Response {
		for rpath, resp := range m {
			if rpath == path {
				return resp
			}
		}
		return Response{Status: 500, Body: "Path not found in response map: " + path}
	}
	s.ResponseFunc(n, f)
}

// Responses prepares the test server to respond the following n requests
// using the provided response parameters.
func (s *TestHTTPServer) Responses(n int, status int, headers map[string]string, body string) {
	f := func(path string) Response {
		return Response{status, headers, body}
	}
	s.ResponseFunc(n, f)
}

// Response prepares the test server to respond the following request
// using the provided response parameters.
func (s *TestHTTPServer) Response(status int, headers map[string]string, body string) {
	s.Responses(1, status, headers, body)
}
