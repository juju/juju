// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
)

var _ = gc.Suite(&ListResourcesSuite{})

type ListResourcesSuite struct {
	BaseSuite
}

func (s *ListResourcesSuite) TestOkay(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res2, apiRes2 := newResource(c, "eggs", "a-user", "...")
	s.data.ReturnListResources = resource.ServiceResources{
		Resources: []resource.Resource{
			res1,
			res2,
		},
	}
	facade := server.NewFacade(s.data)

	results, err := facade.ListResources(api.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.ResourcesResults{
		Results: []api.ResourcesResult{{
			Resources: []api.Resource{
				apiRes1,
				apiRes2,
			},
		}},
	})
	s.stub.CheckCallNames(c, "ListResources")
	s.stub.CheckCall(c, 0, "ListResources", "a-service")
}

func (s *ListResourcesSuite) TestEmpty(c *gc.C) {
	facade := server.NewFacade(s.data)

	results, err := facade.ListResources(api.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.ResourcesResults{
		Results: []api.ResourcesResult{{}},
	})
	s.stub.CheckCallNames(c, "ListResources")
}

func (s *ListResourcesSuite) TestError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	facade := server.NewFacade(s.data)

	results, err := facade.ListResources(api.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.ResourcesResults{
		Results: []api.ResourcesResult{{
			ErrorResult: params.ErrorResult{Error: &params.Error{
				Message: "<failure>",
			}},
		}},
	})
	s.stub.CheckCallNames(c, "ListResources")
}
