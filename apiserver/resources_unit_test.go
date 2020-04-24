// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/state"
)

type UnitResourcesHandlerSuite struct {
	testing.IsolationSuite

	stub     *testing.Stub
	urlStr   string
	recorder *httptest.ResponseRecorder
}

var _ = gc.Suite(&UnitResourcesHandlerSuite{})

func (s *UnitResourcesHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = new(testing.Stub)

	args := url.Values{}
	args.Add(":unit", "foo/0")
	args.Add(":resource", "blob")
	s.urlStr = "https://api:17017/?" + args.Encode()

	s.recorder = httptest.NewRecorder()
}

func (s *UnitResourcesHandlerSuite) closer() bool {
	s.stub.AddCall("Close")
	return false
}

func (s *UnitResourcesHandlerSuite) TestWrongMethod(c *gc.C) {
	handler := &apiserver.UnitResourcesHandler{}

	req, err := http.NewRequest("POST", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	c.Assert(s.recorder.Code, gc.Equals, http.StatusMethodNotAllowed)
	s.stub.CheckNoCalls(c)
}

func (s *UnitResourcesHandlerSuite) TestOpenerCreationError(c *gc.C) {
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
	opener := &stubResourceOpener{
		Stub: s.stub,
	}
	failure, expectedBody := apiFailure("boom", "")
	s.stub.SetErrors(failure)
	handler := &apiserver.UnitResourcesHandler{
		NewOpener: func(_ *http.Request, kinds ...string) (resource.Opener, state.PoolHelper, error) {
			s.stub.AddCall("NewOpener", kinds)
			return opener, apiservertesting.StubPoolHelper{StubRelease: s.closer}, nil
		},
	}

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c, http.StatusInternalServerError, "application/json", expectedBody)
	s.stub.CheckCalls(c, []testing.StubCall{
		{"NewOpener", []interface{}{[]string{names.UnitTagKind, names.ApplicationTagKind}}},
		{"OpenResource", []interface{}{"blob"}},
		{"Close", nil},
	})
}

func (s *UnitResourcesHandlerSuite) TestSuccess(c *gc.C) {
	const body = "some data"
	opened := resourcetesting.NewResource(c, new(testing.Stub), "blob", "app", body)
	opener := &stubResourceOpener{
		Stub:               s.stub,
		ReturnOpenResource: opened,
	}
	handler := &apiserver.UnitResourcesHandler{
		NewOpener: func(_ *http.Request, kinds ...string) (resource.Opener, state.PoolHelper, error) {
			s.stub.AddCall("NewOpener", kinds)
			return opener, apiservertesting.StubPoolHelper{StubRelease: s.closer}, nil
		},
	}

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c, http.StatusOK, "application/octet-stream", body)
	s.stub.CheckCalls(c, []testing.StubCall{
		{"NewOpener", []interface{}{[]string{names.UnitTagKind, names.ApplicationTagKind}}},
		{"OpenResource", []interface{}{"blob"}},
		{"Close", nil},
	})
}
func (s *UnitResourcesHandlerSuite) checkResp(c *gc.C, status int, ctype, body string) {
	checkHTTPResp(c, s.recorder, status, ctype, body)
}

type stubResourceOpener struct {
	*testing.Stub
	ReturnOpenResource resource.Opened
}

func (s *stubResourceOpener) OpenResource(name string) (resource.Opened, error) {
	s.AddCall("OpenResource", name)
	if err := s.NextErr(); err != nil {
		return resource.Opened{}, err
	}
	return s.ReturnOpenResource, nil
}
