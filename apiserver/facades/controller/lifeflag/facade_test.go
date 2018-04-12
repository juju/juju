// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/lifeflag"
	"github.com/juju/juju/apiserver/params"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (*FacadeSuite) TestFacadeAuthFailure(c *gc.C) {
	facade, err := lifeflag.NewFacade(nil, nil, auth(false))
	c.Check(facade, gc.IsNil)
	c.Check(err, gc.Equals, common.ErrPerm)
}

func (*FacadeSuite) TestLifeBadEntity(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Life(entities("archibald snookums"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, gc.Equals, params.Life(""))

	// TODO(fwereade): this is DUMB. should just be a parse error.
	// but I'm not fixing the underlying implementation as well.
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (*FacadeSuite) TestLifeAuthFailure(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Life(entities("unit-foo-1"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, gc.Equals, params.Life(""))
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (*FacadeSuite) TestLifeNotFound(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Life(modelEntity())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, gc.Equals, params.Life(""))
	c.Check(result.Error, jc.Satisfies, params.IsCodeNotFound)
}

func (*FacadeSuite) TestLifeSuccess(c *gc.C) {
	backend := &mockBackend{exist: true}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Check(err, jc.ErrorIsNil)

	results, err := facade.Life(modelEntity())
	c.Check(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{Life: params.Dying}},
	})
}

func (*FacadeSuite) TestWatchBadEntity(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Watch(entities("archibald snookums"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, gc.Equals, "")

	// TODO(fwereade): this is DUMB. should just be a parse error.
	// but I'm not fixing the underlying implementation as well.
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (*FacadeSuite) TestWatchAuthFailure(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Watch(entities("unit-foo-1"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, gc.Equals, "")
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (*FacadeSuite) TestWatchNotFound(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Watch(modelEntity())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, gc.Equals, "")
	c.Check(result.Error, jc.Satisfies, params.IsCodeNotFound)
}

func (*FacadeSuite) TestWatchBadWatcher(c *gc.C) {
	backend := &mockBackend{exist: true}
	facade, err := lifeflag.NewFacade(backend, nil, auth(true))
	c.Check(err, jc.ErrorIsNil)

	results, err := facade.Watch(modelEntity())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, gc.Equals, "")
	c.Check(result.Error, gc.ErrorMatches, "blammo")
}

func (*FacadeSuite) TestWatchSuccess(c *gc.C) {
	backend := &mockBackend{exist: true, watch: true}
	resources := common.NewResources()
	facade, err := lifeflag.NewFacade(backend, resources, auth(true))
	c.Check(err, jc.ErrorIsNil)

	results, err := facade.Watch(modelEntity())
	c.Check(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}},
	})
}
