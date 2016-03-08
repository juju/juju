// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
)

var _ = gc.Suite(&AddPendingResourcesSuite{})

type AddPendingResourcesSuite struct {
	BaseSuite
}

func (s *AddPendingResourcesSuite) TestOkay(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	facade := server.NewFacade(s.data)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "AddPendingResource")
	s.stub.CheckCall(c, 0, "AddPendingResource", "a-service", "", res1.Resource, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestError(c *gc.C) {
	_, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	facade := server.NewFacade(s.data)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "AddPendingResource")
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
		ErrorResult: params.ErrorResult{Error: &params.Error{
			Message: `while adding pending resource info for "spam": <failure>`,
		}},
	})
}
