// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	lister *stubSpecLister
}

func (s *specSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.lister = &stubSpecLister{stub: s.stub}
}

func (s *specSuite) TestListSpecsOkay(c *gc.C) {
	spec1, apiSpec1 := newSpec(c, "spam")
	spec2, apiSpec2 := newSpec(c, "eggs")
	s.lister.ReturnSpecs = []resource.Spec{
		spec1,
		spec2,
	}
	facade := server.NewFacade(s.lister)

	apiSpecs, err := facade.ListSpecs(api.ListSpecsArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(apiSpecs, jc.DeepEquals, api.ResourceSpecsResults{
		Results: []api.ResourceSpecsResult{{
			Specs: []api.ResourceSpec{
				apiSpec1,
				apiSpec2,
			},
		}},
	})
	c.Check(s.stub.Calls(), gc.HasLen, 1)
	s.stub.CheckCall(c, 0, "ListResourceSpecs", "a-service")
}

func (s *specSuite) TestListSpecsEmpty(c *gc.C) {
	facade := server.NewFacade(s.lister)

	apiSpecs, err := facade.ListSpecs(api.ListSpecsArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(apiSpecs, jc.DeepEquals, api.ResourceSpecsResults{
		Results: []api.ResourceSpecsResult{{
			Specs: nil,
		}},
	})
	s.stub.CheckCallNames(c, "ListResourceSpecs")
}

func (s *specSuite) TestListSpecsError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	facade := server.NewFacade(s.lister)

	results, err := facade.ListSpecs(api.ListSpecsArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.ResourceSpecsResults{
		Results: []api.ResourceSpecsResult{{
			ErrorResult: params.ErrorResult{Error: &params.Error{
				Message: "<failure>",
			}},
		}},
	})
	s.stub.CheckCallNames(c, "ListResourceSpecs")
}

func newSpec(c *gc.C, name string) (resource.Spec, api.ResourceSpec) {
	info := charmresource.Info{
		Name: name,
		Type: charmresource.TypeFile,
		Path: name + ".tgz",
	}
	spec := resource.Spec{
		Definition: info,
		Origin:     resource.OriginKindUpload,
		Revision:   resource.NoRevision,
	}
	err := spec.Validate()
	c.Assert(err, jc.ErrorIsNil)

	apiSpec := api.ResourceSpec{
		Name:   name,
		Type:   "file",
		Path:   name + ".tgz",
		Origin: "upload",
	}

	return spec, apiSpec
}

type stubSpecLister struct {
	stub *testing.Stub

	ReturnSpecs []resource.Spec
}

func (s *stubSpecLister) ListResourceSpecs(service string) ([]resource.Spec, error) {
	s.stub.AddCall("ListResourceSpecs", service)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnSpecs, nil
}
