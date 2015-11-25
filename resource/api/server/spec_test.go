// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/apitesting"
	"github.com/juju/juju/resource/api/server"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.IsolationSuite

	stub  *testing.Stub
	state *stubSpecState
}

func (s *specSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.state = &stubSpecState{stub: s.stub}
}

func (s *specSuite) TestListSpecsOkay(c *gc.C) {
	spec1, apiSpec1 := apitesting.NewSpec(c, "spam")
	spec2, apiSpec2 := apitesting.NewSpec(c, "eggs")
	s.state.ReturnSpecs = []resource.Spec{
		spec1,
		spec2,
	}
	facade := server.NewFacade(s.state)

	apiSpecs, err := facade.ListSpecs(api.ListSpecsArgs{
		Service: "service-a-service",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(apiSpecs, jc.DeepEquals, api.ListSpecsResults{
		Results: []api.ResourceSpec{
			apiSpec1,
			apiSpec2,
		},
	})
	c.Check(s.stub.Calls(), gc.HasLen, 1)
	s.stub.CheckCall(c, 0, "ListResourceSpecs", "a-service")
}

func (s *specSuite) TestListSpecsEmpty(c *gc.C) {
	facade := server.NewFacade(s.state)

	apiSpecs, err := facade.ListSpecs(api.ListSpecsArgs{
		Service: "service-a-service",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(apiSpecs, jc.DeepEquals, api.ListSpecsResults{})
	s.stub.CheckCallNames(c, "ListResourceSpecs")
}

func (s *specSuite) TestListSpecsBadTag(c *gc.C) {
	facade := server.NewFacade(s.state)

	_, err := facade.ListSpecs(api.ListSpecsArgs{
		Service: "a-service",
	})

	c.Check(err, gc.NotNil)
	s.stub.CheckNoCalls(c)
}

func (s *specSuite) TestListSpecsError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	facade := server.NewFacade(s.state)

	_, err := facade.ListSpecs(api.ListSpecsArgs{
		Service: "service-a-service",
	})

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "ListResourceSpecs")
}

type stubSpecState struct {
	stub *testing.Stub

	ReturnSpecs []resource.Spec
}

func (s *stubSpecState) ListResourceSpecs(service string) ([]resource.Spec, error) {
	s.stub.AddCall("ListResourceSpecs", service)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnSpecs, nil
}
