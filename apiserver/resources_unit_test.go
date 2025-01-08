// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type UnitResourcesHandlerSuite struct {
	testing.IsolationSuite

	opener *MockOpener

	urlStr   string
	recorder *httptest.ResponseRecorder
}

var _ = gc.Suite(&UnitResourcesHandlerSuite{})

func (s *UnitResourcesHandlerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.opener = NewMockOpener(ctrl)
	return ctrl
}

func (s *UnitResourcesHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	args := url.Values{}
	args.Add(":unit", "foo/0")
	args.Add(":resource", "blob")
	s.urlStr = "https://api:17017/?" + args.Encode()

	s.recorder = httptest.NewRecorder()
}

func (s *UnitResourcesHandlerSuite) TestWrongMethod(c *gc.C) {
	defer s.setupMocks(c).Finish()
	handler := &apiserver.UnitResourcesHandler{}

	req, err := http.NewRequest("POST", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	c.Assert(s.recorder.Code, gc.Equals, http.StatusMethodNotAllowed)
}

func (s *UnitResourcesHandlerSuite) TestOpenerCreationError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	failure, expectedBody := apiFailure("boom", "")
	handler := &apiserver.UnitResourcesHandler{
		NewOpener: func(_ *http.Request, kinds ...string) (resource.Opener, state.PoolHelper, error) {
			return nil, nil, failure
		},
	}

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c,
		http.StatusInternalServerError,
		"application/json",
		expectedBody,
	)
}

func (s *UnitResourcesHandlerSuite) TestOpenResourceError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	failure, expectedBody := apiFailure("boom", "")
	s.opener.EXPECT().OpenResource(gomock.Any(), "blob").Return(resource.Opened{}, failure)
	handler := &apiserver.UnitResourcesHandler{
		NewOpener: func(_ *http.Request, kinds ...string) (resource.Opener, state.PoolHelper, error) {
			return s.opener, apiservertesting.StubPoolHelper{StubRelease: func() bool { return true }}, nil
		},
	}

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c, http.StatusInternalServerError, "application/json", expectedBody)
}

func (s *UnitResourcesHandlerSuite) TestSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	const body = "some data"
	opened := resourcetesting.NewResource(c, new(testing.Stub), "blob", "app", body)
	handler := &apiserver.UnitResourcesHandler{
		NewOpener: func(_ *http.Request, kinds ...string) (resource.Opener, state.PoolHelper, error) {
			return s.opener, apiservertesting.StubPoolHelper{StubRelease: func() bool { return true }}, nil
		},
	}

	s.opener.EXPECT().OpenResource(gomock.Any(), "blob").Return(opened, nil)
	s.opener.EXPECT().SetResourceUsed(gomock.Any(), "blob").Return(nil)

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c, http.StatusOK, "application/octet-stream", body)
}

func (s *UnitResourcesHandlerSuite) checkResp(c *gc.C, status int, ctype, body string) {
	c.Assert(s.recorder.Code, gc.Equals, status)
	hdr := s.recorder.Header()
	c.Check(hdr.Get("Content-Type"), gc.Equals, ctype)
	c.Check(hdr.Get("Content-Length"), gc.Equals, strconv.Itoa(len(body)))

	actualBody, err := io.ReadAll(s.recorder.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(actualBody), gc.Equals, body)
}

func apiFailure(msg, code string) (error, string) {
	failure := errors.New(msg)
	data := mustMarshalJSON(params.ErrorResult{
		Error: &params.Error{
			Message: msg,
			Code:    code,
		},
	})
	return failure, string(data)
}

func mustMarshalJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
