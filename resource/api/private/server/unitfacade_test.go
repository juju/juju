// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/private"
	"github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/resourcetesting"
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

	uf := server.NewUnitFacade(expected)

	s.stub.CheckNoCalls(c)
	c.Check(uf.DataStore, gc.Equals, expected)
}

func (s *UnitFacadeSuite) TestGetResourceInfoOkay(c *gc.C) {
	opened1 := resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	res1 := opened1.Resource
	opened2 := resourcetesting.NewResource(c, s.stub, "eggs", "a-service", "other data")
	res2 := opened2.Resource
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resource.ServiceResources{
		Resources: []resource.Resource{res1, res2},
	}
	uf := server.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(private.ListResourcesArgs{
		ResourceNames: []string{"spam", "eggs"},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, jc.DeepEquals, private.ResourcesResult{
		Resources: []private.ResourceResult{{
			Resource: api.Resource2API(res1),
		}, {
			Resource: api.Resource2API(res2),
		}},
	})
}

func (s *UnitFacadeSuite) TestGetResourceInfoEmpty(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resource.ServiceResources{
		Resources: []resource.Resource{opened.Resource},
	}
	uf := server.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(private.ListResourcesArgs{
		ResourceNames: []string{},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, jc.DeepEquals, private.ResourcesResult{
		Resources: []private.ResourceResult{},
	})
}

func (s *UnitFacadeSuite) TestGetResourceInfoNotFound(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	store := &stubUnitDataStore{Stub: s.stub}
	store.ReturnListResources = resource.ServiceResources{
		Resources: []resource.Resource{opened.Resource},
	}
	uf := server.UnitFacade{DataStore: store}

	results, err := uf.GetResourceInfo(private.ListResourcesArgs{
		ResourceNames: []string{"eggs"},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListResources")
	c.Check(results, jc.DeepEquals, private.ResourcesResult{
		Resources: []private.ResourceResult{{
			ErrorResult: params.ErrorResult{
				Error: common.ServerError(errors.NotFoundf(`resource "eggs"`)),
			},
		}},
	})
}
