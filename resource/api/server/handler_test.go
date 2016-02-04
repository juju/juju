// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
)

type LegacyHTTPHandlerSuite struct {
	BaseSuite

	username string
	req      *http.Request
	header   http.Header
	resp     *stubHTTPResponseWriter
	result   *api.UploadResult
}

var _ = gc.Suite(&LegacyHTTPHandlerSuite{})

func (s *LegacyHTTPHandlerSuite) SetUpTest(c *gc.C) {
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
	s.result = &api.UploadResult{}
}

func (s *LegacyHTTPHandlerSuite) connect(req *http.Request) (server.DataStore, names.Tag, error) {
	s.stub.AddCall("Connect", req)
	if err := s.stub.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	tag := names.NewUserTag(s.username)
	return s.data, tag, nil
}

func (s *LegacyHTTPHandlerSuite) handleUpload(username string, st server.DataStore, req *http.Request) (*api.UploadResult, error) {
	s.stub.AddCall("HandleUpload", username, st, req)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.result, nil
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPConnectFailure(c *gc.C) {
	s.username = "youknowwho"
	handler := server.LegacyHTTPHandler{
		Connect:      s.connect,
		HandleUpload: s.handleUpload,
	}
	copied := *s.req
	req := &copied
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
	c.Check(req, jc.DeepEquals, s.req) // did not change
	c.Check(s.header, jc.DeepEquals, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{strconv.Itoa(len(expected))},
	})
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPUnsupportedMethod(c *gc.C) {
	s.username = "youknowwho"
	handler := server.LegacyHTTPHandler{
		Connect:      s.connect,
		HandleUpload: s.handleUpload,
	}
	s.req.Method = "POST"
	copied := *s.req
	req := &copied
	_, expected := apiFailure(c, `unsupported method: "POST"`, params.CodeMethodNotAllowed)

	handler.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"Header",
		"Header",
		"WriteHeader",
		"Write",
	)
	s.stub.CheckCall(c, 0, "Connect", req)
	s.stub.CheckCall(c, 3, "WriteHeader", http.StatusMethodNotAllowed)
	s.stub.CheckCall(c, 4, "Write", expected)
	c.Check(req, jc.DeepEquals, s.req) // did not change
	c.Check(s.header, jc.DeepEquals, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{strconv.Itoa(len(expected))},
	})
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPPutSuccess(c *gc.C) {
	s.result.Resource.Name = "spam"
	expected, err := json.Marshal(s.result)
	c.Assert(err, jc.ErrorIsNil)
	s.username = "youknowwho"
	handler := server.LegacyHTTPHandler{
		Connect:      s.connect,
		HandleUpload: s.handleUpload,
	}
	s.req.Method = "PUT"
	copied := *s.req
	req := &copied

	handler.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"HandleUpload",
		"Header",
		"Header",
		"WriteHeader",
		"Write",
	)
	s.stub.CheckCall(c, 0, "Connect", req)
	s.stub.CheckCall(c, 1, "HandleUpload", "youknowwho", s.data, req)
	s.stub.CheckCall(c, 4, "WriteHeader", http.StatusOK)
	s.stub.CheckCall(c, 5, "Write", string(expected))
	c.Check(req, jc.DeepEquals, s.req) // did not change
	c.Check(s.header, jc.DeepEquals, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{fmt.Sprint(len(expected))},
	})
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPPutHandleUploadFailure(c *gc.C) {
	s.username = "youknowwho"
	handler := server.LegacyHTTPHandler{
		Connect:      s.connect,
		HandleUpload: s.handleUpload,
	}
	s.req.Method = "PUT"
	copied := *s.req
	req := &copied
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
	)
	s.stub.CheckCall(c, 0, "Connect", req)
	s.stub.CheckCall(c, 1, "HandleUpload", "youknowwho", s.data, req)
	s.stub.CheckCall(c, 4, "WriteHeader", http.StatusInternalServerError)
	s.stub.CheckCall(c, 5, "Write", expected)
	c.Check(req, jc.DeepEquals, s.req) // did not change
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
