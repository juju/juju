// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/juju/tc"
)

type HTTPSuite struct{}

var Server = NewHTTPServer(5 * time.Second)

func (s *HTTPSuite) SetUpSuite(c *tc.C) {
	Server.Start()
}
func (s *HTTPSuite) TearDownSuite(c *tc.C) {}

func (s *HTTPSuite) SetUpTest(c *tc.C) {}

func (s *HTTPSuite) TearDownTest(c *tc.C) {
	Server.Flush()
}

func (s *HTTPSuite) URL(path string) string {
	return Server.URL + path
}

type HTTPServer struct {
	URL      string
	Timeout  time.Duration
	started  bool
	request  chan *http.Request
	response chan ResponseFunc
}

func NewHTTPServer(timeout time.Duration) *HTTPServer {
	return &HTTPServer{Timeout: timeout}
}

type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

type ResponseFunc func(path string) Response

func (s *HTTPServer) Start() {
	if s.started {
		return
	}
	s.started = true
	s.request = make(chan *http.Request, 64)
	s.response = make(chan ResponseFunc, 64)

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	s.URL = fmt.Sprintf("http://localhost:%d", port)
	go func() { _ = http.Serve(l, s) }()

	s.Response(203, nil, nil)
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

// Flush discards all pending requests and responses.
func (s *HTTPServer) Flush() {
	for {
		select {
		case <-s.request:
		case <-s.response:
		default:
			return
		}
	}
}

func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	_ = req.ParseMultipartForm(1e6)
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
		resp = Response{500, nil, []byte(msg)}
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
	_, _ = w.Write(resp.Body)
}

// WaitRequests returns the next n requests made to the http server from
// the queue. If not enough requests were previously made, it waits until
// the timeout value for them to be made.
func (s *HTTPServer) WaitRequests(n int) []*http.Request {
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
func (s *HTTPServer) WaitRequest() *http.Request {
	return s.WaitRequests(1)[0]
}

// ResponseFunc prepares the test server to respond the following n
// requests using f to build each response.
func (s *HTTPServer) ResponseFunc(n int, f ResponseFunc) {
	for i := 0; i < n; i++ {
		s.response <- f
	}
}

// ResponseMap maps request paths to responses.
type ResponseMap map[string]Response

// ResponseMap prepares the test server to respond the following n
// requests using the m to obtain the responses.
func (s *HTTPServer) ResponseMap(n int, m ResponseMap) {
	f := func(path string) Response {
		for rpath, resp := range m {
			if rpath == path {
				return resp
			}
		}
		body := []byte("Path not found in response map: " + path)
		return Response{Status: 500, Body: body}
	}
	s.ResponseFunc(n, f)
}

// Responses prepares the test server to respond the following n requests
// using the provided response parameters.
func (s *HTTPServer) Responses(n int, status int, headers map[string]string, body []byte) {
	f := func(path string) Response {
		return Response{status, headers, body}
	}
	s.ResponseFunc(n, f)
}

// Response prepares the test server to respond the following request
// using the provided response parameters.
func (s *HTTPServer) Response(status int, headers map[string]string, body []byte) {
	s.Responses(1, status, headers, body)
}
