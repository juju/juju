// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/client"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.IsolationSuite

	stub    *testing.Stub
	facade  *stubFacade
	apiSpec api.ResourceSpec
}

func (s *specSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.facade = &stubFacade{stub: s.stub}
	s.apiSpec = api.ResourceSpec{
		Name:     "spam",
		Type:     "file",
		Path:     "spam.tgz",
		Comment:  "you need it",
		Origin:   "upload",
		Revision: "",
	}
}

func (s *specSuite) TestListSpecOkay(c *gc.C) {
	s.facade.FacadeCallFn = func(_ string, _, response interface{}) error {
		typedResponse, ok := response.(*api.ResourceSpecsResults)
		c.Assert(ok, gc.Equals, true)
		typedResponse.Results = append(typedResponse.Results, api.ResourceSpecsResult{
			Entity: params.Entity{Tag: "service-a-service"},
			Specs:  []api.ResourceSpec{s.apiSpec},
		})
		return nil
	}

	cl := client.NewClient(s.facade)

	specs, err := cl.ListSpecs("a-service")
	c.Assert(err, jc.ErrorIsNil)

	info := charmresource.Info{
		Name:    "spam",
		Type:    charmresource.TypeFile,
		Path:    "spam.tgz",
		Comment: "you need it",
	}
	expected := resource.Spec{
		Definition: info,
		Origin:     resource.OriginUpload,
		Revision:   resource.NoRevision,
	}
	err = expected.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(specs, jc.DeepEquals, [][]resource.Spec{{
		expected,
	}})
	c.Check(s.stub.Calls(), gc.HasLen, 1)
	s.stub.CheckCall(c, 0, "FacadeCall",
		"ListSpecs",
		&api.ListSpecsArgs{
			Entities: []params.Entity{{
				Tag: "service-a-service",
			}},
		},
		&api.ResourceSpecsResults{
			Results: []api.ResourceSpecsResult{{
				Entity: params.Entity{Tag: "service-a-service"},
				Specs:  []api.ResourceSpec{s.apiSpec},
			}},
		},
	)
}

// TODO(ericsnow) Move this to a common testing package.

type stubFacade struct {
	stub         *testing.Stub
	FacadeCallFn func(name string, params, response interface{}) error
}

func (s *stubFacade) FacadeCall(request string, params, response interface{}) error {
	s.stub.AddCall("FacadeCall", request, params, response)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if s.FacadeCallFn != nil {
		return s.FacadeCallFn(request, params, response)
	}
	return nil
}

func (s *stubFacade) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
