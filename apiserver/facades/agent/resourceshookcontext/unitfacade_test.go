// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	api "github.com/juju/juju/api/client/resources"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/agent/resourceshookcontext"
	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&UnitFacadeSuite{})

type UnitFacadeSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
}

func (s *UnitFacadeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
}

func (s *UnitFacadeSuite) TestNewUnitFacade(c *gc.C) {
	expected := &stubUnitDataStore{Stub: s.stub}

	uf := resourceshookcontext.NewUnitFacade(expected)

	s.stub.CheckNoCalls(c)
	c.Check(uf.DataStore, gc.Equals, expected)
}

func (s *UnitFacadeSuite) TestGetResourceInfoOkay(c *gc.C) {
	opened1 := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	res1 := opened1.Resource
	opened2 := resourcetesting.NewResource(c, s.stub, "eggs", "a-application", "other data")
	res2 := opened2.Resource
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resources.ApplicationResources{
		Resources: []resources.Resource{res1, res2},
	}
	uf := resourceshookcontext.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(context.Background(), params.ListUnitResourcesArgs{
		ResourceNames: []string{"spam", "eggs"},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, jc.DeepEquals, params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{{
			Resource: api.Resource2API(res1),
		}, {
			Resource: api.Resource2API(res2),
		}},
	})
}

func (s *UnitFacadeSuite) TestGetResourceInfoEmpty(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resources.ApplicationResources{
		Resources: []resources.Resource{opened.Resource},
	}
	uf := resourceshookcontext.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(context.Background(), params.ListUnitResourcesArgs{
		ResourceNames: []string{},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, jc.DeepEquals, params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{},
	})
}

func (s *UnitFacadeSuite) TestGetResourceInfoNotFound(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-application", "some data")
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resources.ApplicationResources{
		Resources: []resources.Resource{opened.Resource},
	}
	uf := resourceshookcontext.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(context.Background(), params.ListUnitResourcesArgs{
		ResourceNames: []string{"eggs"},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, jc.DeepEquals, params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{{
			ErrorResult: params.ErrorResult{
				Error: apiservererrors.ServerError(errors.NotFoundf(`resource "eggs"`)),
			},
		}},
	})
}
