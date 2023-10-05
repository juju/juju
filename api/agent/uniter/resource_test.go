// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&ResourcesFacadeClientSuite{})

type ResourcesFacadeClientSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	api  *stubAPI
}

func (s *ResourcesFacadeClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.api = &stubAPI{Stub: s.stub}
}

func (s *ResourcesFacadeClientSuite) TestGetResource(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	s.api.setResource(opened.Resource, opened)
	cl, err := uniter.NewResourcesFacadeClient(s.api, names.NewUnitTag("unit/0"))
	c.Assert(err, jc.ErrorIsNil)
	cl.HTTPDoer = s.api

	info, content, err := cl.GetResource("spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Do", "GetResourceInfo")
	c.Check(info, jc.DeepEquals, opened.Resource)
	c.Check(content, jc.DeepEquals, opened)
}

func (s *ResourcesFacadeClientSuite) TestUnitDoer(c *gc.C) {
	body := filetesting.NewStubFile(s.stub, nil)
	req, err := http.NewRequest("GET", "/resources/eggs", body)
	c.Assert(err, jc.ErrorIsNil)
	var resp *http.Response
	doer := uniter.NewUnitHTTPClient(context.Background(), s.api, "spam/1")

	err = doer.Do(context.Background(), req, &resp)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Do")
	//s.stub.CheckCall(c, 0, "Do", expected, body, resp)
	c.Check(req.URL.Path, gc.Equals, "/units/spam/1/resources/eggs")
}

type stubAPI struct {
	base.APICaller
	*testing.Stub

	ReturnFacadeCall params.UnitResourcesResult
	ReturnUnit       string
	ReturnDo         *http.Response
}

func (s *stubAPI) setResource(info resources.Resource, reader io.ReadCloser) {
	s.ReturnFacadeCall = params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{{
			Resource: api.Resource2API(info),
		}},
	}
	s.ReturnDo = &http.Response{
		Body: reader,
	}
}

func (s *stubAPI) Context() context.Context {
	return context.TODO()
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
