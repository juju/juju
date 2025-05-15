// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testhelpers/filetesting"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&ResourcesFacadeClientSuite{})

type ResourcesFacadeClientSuite struct {
	testhelpers.IsolationSuite

	stub *testhelpers.Stub
	api  *stubAPI
}

func (s *ResourcesFacadeClientSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testhelpers.Stub{}
	s.api = &stubAPI{Stub: s.stub}
}

func (s *ResourcesFacadeClientSuite) TestGetResource(c *tc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	s.api.setResource(opened.Resource, opened)
	cl, err := uniter.NewResourcesFacadeClient(s.api, names.NewUnitTag("unit/0"))
	c.Assert(err, tc.ErrorIsNil)
	cl.HTTPDoer = s.api

	info, content, err := cl.GetResource(c.Context(), "spam")
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Do", "GetResourceInfo")
	c.Check(info, tc.DeepEquals, opened.Resource)
	c.Check(content, tc.DeepEquals, opened)
}

func (s *ResourcesFacadeClientSuite) TestUnitDoer(c *tc.C) {
	body := filetesting.NewStubFile(s.stub, nil)
	req, err := http.NewRequest("GET", "/resources/eggs", body)
	c.Assert(err, tc.ErrorIsNil)
	var resp *http.Response
	doer := uniter.NewUnitHTTPClient(s.api, "spam/1")

	err = doer.Do(c.Context(), req, &resp)
	c.Assert(err, tc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Do")
	//s.stub.CheckCall(c, 0, "Do", expected, body, resp)
	c.Check(req.URL.Path, tc.Equals, "/units/spam/1/resources/eggs")
}

type stubAPI struct {
	base.APICaller
	*testhelpers.Stub

	ReturnFacadeCall params.UnitResourcesResult
	ReturnUnit       string
	ReturnDo         *http.Response
}

func (s *stubAPI) setResource(info resource.Resource, reader io.ReadCloser) {
	s.ReturnFacadeCall = params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{{
			Resource: api.Resource2API(info),
		}},
	}
	s.ReturnDo = &http.Response{
		Body: reader,
	}
}

func (s *stubAPI) BestFacadeVersion(_ string) int {
	return 1
}

func (s *stubAPI) HTTPClient() (*httprequest.Client, error) {
	return &httprequest.Client{
		//Doer: func,
	}, nil
}

func (s *stubAPI) APICall(ctx context.Context, objType string, version int, id, request string, args, response interface{}) error {
	s.AddCall(request, args, response)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	resp := response.(*params.UnitResourcesResult)
	*resp = s.ReturnFacadeCall
	return nil
}

func (s *stubAPI) Unit() string {
	s.AddCall("Unit")
	s.NextErr() // Pop one off.

	return s.ReturnUnit
}

func (s *stubAPI) Do(ctx context.Context, req *http.Request, response interface{}) error {
	s.AddCall("Do", req, response)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	resp := response.(**http.Response)
	*resp = s.ReturnDo
	return nil
}
