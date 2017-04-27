// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
)

const downloadContent = "body"

type HTTPHandlerSuite struct {
	BaseSuite

	username     string
	req          *http.Request
	header       http.Header
	resp         *stubHTTPResponseWriter
	uploadResult *api.UploadResult
}

var _ = gc.Suite(&HTTPHandlerSuite{})

func (s *HTTPHandlerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	method := "..."
	urlStr := "..."
	body := strings.NewReader("...")
	req, err := http.NewRequest(method, urlStr, body)
	c.Assert(err, jc.ErrorIsNil)

	s.req = req
	s.header = make(http.Header)
	s.resp = &stubHTTPResponseWriter{
		stub:         s.stub,
		returnHeader: s.header,
	}
	s.uploadResult = &api.UploadResult{}
}

func (s *HTTPHandlerSuite) connect(req *http.Request) (server.DataStore, server.Closer, names.Tag, error) {
	s.stub.AddCall("Connect", req)
	if err := s.stub.NextErr(); err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	closer := func() error {
		s.stub.AddCall("Close")
		return s.stub.NextErr()
	}
	tag := names.NewUserTag(s.username)
	return s.data, closer, tag, nil
}

func (s *HTTPHandlerSuite) handleDownload(st server.DataStore, req *http.Request) (io.ReadCloser, int64, error) {
	s.stub.AddCall("HandleDownload", st, req)
	if err := s.stub.NextErr(); err != nil {
		return nil, 0, errors.Trace(err)
	}

	reader := ioutil.NopCloser(strings.NewReader(downloadContent))
	return reader, int64(len(downloadContent)), nil
}

func (s *HTTPHandlerSuite) handleUpload(username string, st server.DataStore, req *http.Request) (*api.UploadResult, error) {
	s.stub.AddCall("HandleUpload", username, st, req)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return s.uploadResult, nil
}

func (s *HTTPHandlerSuite) copyRequest() *http.Request {
	copy := *s.req
	return &copy
}

func (s *HTTPHandlerSuite) TestServeHTTPConnectFailure(c *gc.C) {
	s.username = "youknowwho"
	handler := server.HTTPHandler{
		Connect: s.connect,
	}
	req := s.copyRequest()
	failure, expected := apiFailure(c, "<failure>", "")
	s.stub.SetErrors(failure)

	handler.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"Header",
		"Header",
		"WriteHeader",
		"Write",
	)
	s.stub.CheckCall(c, 0, "Connect", req)
	s.stub.CheckCall(c, 3, "WriteHeader", http.StatusInternalServerError)
	s.stub.CheckCall(c, 4, "Write", expected)
	c.Check(s.header, jc.DeepEquals, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{strconv.Itoa(len(expected))},
	})
}

func (s *HTTPHandlerSuite) TestServeHTTPUnsupportedMethod(c *gc.C) {
	s.username = "youknowwho"
	handler := server.HTTPHandler{
		Connect: s.connect,
	}
	s.req.Method = "POST"
	req := s.copyRequest()
	_, expected := apiFailure(c, `unsupported method: "POST"`, params.CodeMethodNotAllowed)

	handler.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"Header",
		"Header",
		"WriteHeader",
		"Write",
		"Close",
	)
	s.stub.CheckCall(c, 0, "Connect", req)
	s.stub.CheckCall(c, 3, "WriteHeader", http.StatusMethodNotAllowed)
	s.stub.CheckCall(c, 4, "Write", expected)
	c.Check(s.header, jc.DeepEquals, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{strconv.Itoa(len(expected))},
	})
}

func (s *HTTPHandlerSuite) TestServeHTTPGetSuccess(c *gc.C) {
	handler := server.HTTPHandler{
		Connect:        s.connect,
		HandleDownload: s.handleDownload,
	}
	s.req.Method = "GET"
	req := s.copyRequest()

	handler.ServeHTTP(s.resp, req)

	s.stub.CheckCalls(c, []testing.StubCall{
		{"Connect", []interface{}{req}},
		{"HandleDownload", []interface{}{s.data, req}},
		{"Header", []interface{}{}},
		{"WriteHeader", []interface{}{http.StatusOK}},
		{"Write", []interface{}{downloadContent}},
		{"Close", nil},
	})
	c.Check(s.header, jc.DeepEquals, http.Header{
		"Content-Type":   []string{"application/octet-stream"},
		"Content-Length": []string{fmt.Sprint(len(downloadContent))},
	})
}

func (s *HTTPHandlerSuite) TestServeHTTPPutSuccess(c *gc.C) {
	s.uploadResult.Resource.Name = "spam"
	expected, err := json.Marshal(s.uploadResult)
	c.Assert(err, jc.ErrorIsNil)
	s.username = "youknowwho"
	handler := server.HTTPHandler{
		Connect:      s.connect,
		HandleUpload: s.handleUpload,
	}
	s.req.Method = "PUT"
	req := s.copyRequest()

	handler.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"HandleUpload",
		"Header",
		"Header",
		"WriteHeader",
		"Write",
		"Close",
	)
	s.stub.CheckCall(c, 0, "Connect", req)
	s.stub.CheckCall(c, 1, "HandleUpload", "youknowwho", s.data, req)
	s.stub.CheckCall(c, 4, "WriteHeader", http.StatusOK)
	s.stub.CheckCall(c, 5, "Write", string(expected))
	c.Check(s.header, jc.DeepEquals, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{fmt.Sprint(len(expected))},
	})
}

func (s *HTTPHandlerSuite) TestServeHTTPPutHandleUploadFailure(c *gc.C) {
	s.username = "youknowwho"
	handler := server.HTTPHandler{
		Connect:      s.connect,
		HandleUpload: s.handleUpload,
	}
	s.req.Method = "PUT"
	req := s.copyRequest()
	failure, expected := apiFailure(c, "<failure>", "")
	s.stub.SetErrors(nil, failure)

	handler.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"HandleUpload",
		"Header",
		"Header",
		"WriteHeader",
		"Write",
		"Close",
	)
	s.stub.CheckCall(c, 0, "Connect", req)
	s.stub.CheckCall(c, 1, "HandleUpload", "youknowwho", s.data, req)
	s.stub.CheckCall(c, 4, "WriteHeader", http.StatusInternalServerError)
	s.stub.CheckCall(c, 5, "Write", expected)
	c.Check(s.header, jc.DeepEquals, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{strconv.Itoa(len(expected))},
	})
}

func apiFailure(c *gc.C, msg, code string) (error, string) {
	failure := errors.New(msg)

	data, err := json.Marshal(params.ErrorResult{
		Error: &params.Error{
			Message: msg,
			Code:    code,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	return failure, string(data)
}

type stubHTTPResponseWriter struct {
	stub *testing.Stub

	returnHeader http.Header
}

func (s *stubHTTPResponseWriter) Header() http.Header {
	s.stub.AddCall("Header")
	s.stub.NextErr() // Pop one off.

	return s.returnHeader
}

func (s *stubHTTPResponseWriter) Write(data []byte) (int, error) {
	s.stub.AddCall("Write", string(data))
	if err := s.stub.NextErr(); err != nil {
		return 0, errors.Trace(err)
	}

	return len(data), nil
}

func (s *stubHTTPResponseWriter) WriteHeader(code int) {
	s.stub.AddCall("WriteHeader", code)
	s.stub.NextErr() // Pop one off.
}
