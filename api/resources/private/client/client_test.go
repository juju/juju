// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	api "github.com/juju/juju/api/resources"
	"github.com/juju/juju/api/resources/private/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
)

var _ = gc.Suite(&UnitFacadeClientSuite{})

type UnitFacadeClientSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	api  *stubAPI
}

func (s *UnitFacadeClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.api = &stubAPI{Stub: s.stub}
}

func (s *UnitFacadeClientSuite) TestNewUnitFacadeClient(c *gc.C) {
	caller := &stubAPI{Stub: s.stub}
	doer := &stubAPI{Stub: s.stub}

	cl := client.NewUnitFacadeClient(context.Background(), caller, doer)

	s.stub.CheckNoCalls(c)
	c.Check(cl.FacadeCaller, gc.Equals, caller)
	c.Check(cl.HTTPClient, gc.Equals, doer)
}

func (s *UnitFacadeClientSuite) TestGetResource(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	s.api.setResource(opened.Resource, opened)
	cl := client.NewUnitFacadeClient(context.Background(), s.api, s.api)

	info, content, err := cl.GetResource("spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Do", "FacadeCall")
	c.Check(info, jc.DeepEquals, opened.Resource)
	c.Check(content, jc.DeepEquals, opened)
}

func (s *UnitFacadeClientSuite) TestUnitDoer(c *gc.C) {
	body := filetesting.NewStubFile(s.stub, nil)
	req, err := http.NewRequest("GET", "/resources/eggs", body)
	c.Assert(err, jc.ErrorIsNil)
	var resp *http.Response
	doer := client.NewUnitHTTPClient(context.Background(), s.api, "spam/1")

	err = doer.Do(context.Background(), req, &resp)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Do")
	//s.stub.CheckCall(c, 0, "Do", expected, body, resp)
	c.Check(req.URL.Path, gc.Equals, "/units/spam/1/resources/eggs")
}

type stubAPI struct {
	*testing.Stub

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

func (s *stubAPI) FacadeCall(request string, args, response interface{}) error {
	s.AddCall("FacadeCall", args, response)
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
