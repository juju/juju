// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/lifeflag"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testing.IsolationSuite

	modelUUID       coremodel.UUID
	watcherRegistry *MockWatcherRegistry
}

var _ = gc.Suite(&FacadeSuite{})

func (s *FacadeSuite) SetUpTest(c *gc.C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
}

func (s *FacadeSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)
	return ctrl
}

func (s *FacadeSuite) TestFacadeAuthFailure(c *gc.C) {
	facade, err := lifeflag.NewFacade(s.modelUUID, nil, nil, auth(false))
	c.Check(facade, gc.IsNil)
	c.Check(err, gc.Equals, apiservererrors.ErrPerm)
}

func (s *FacadeSuite) TestLifeBadEntity(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Life(context.Background(), entities("archibald snookums"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, gc.Equals, life.Value(""))

	// TODO(fwereade): this is DUMB. should just be a parse error.
	// but I'm not fixing the underlying implementation as well.
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestLifeAuthFailure(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Life(context.Background(), entities("unit-foo-1"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, gc.Equals, life.Value(""))
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestLifeNotFound(c *gc.C) {
	backend := &mockBackend{entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Life(context.Background(), modelEntity(s.modelUUID))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, gc.Equals, life.Value(""))
	c.Check(result.Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *FacadeSuite) TestLifeSuccess(c *gc.C) {
	backend := &mockBackend{exist: true, entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Check(err, jc.ErrorIsNil)

	results, err := facade.Life(context.Background(), modelEntity(s.modelUUID))
	c.Check(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{Life: life.Dying}},
	})
}

func (s *FacadeSuite) TestWatchBadEntity(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Watch(context.Background(), entities("archibald snookums"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, gc.Equals, "")

	// TODO(fwereade): this is DUMB. should just be a parse error.
	// but I'm not fixing the underlying implementation as well.
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestWatchAuthFailure(c *gc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Watch(context.Background(), entities("unit-foo-1"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, gc.Equals, "")
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestWatchNotFound(c *gc.C) {
	backend := &mockBackend{exist: false, watch: true, entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.Watch(context.Background(), modelEntity(s.modelUUID))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, gc.Equals, "")
	c.Check(result.Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *FacadeSuite) TestWatchBadWatcher(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	backend := &mockBackend{exist: true, watch: false, entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, s.watcherRegistry, auth(true))
	c.Check(err, jc.ErrorIsNil)

	results, err := facade.Watch(context.Background(), modelEntity(s.modelUUID))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, gc.Equals, "")
	c.Check(result.Error, gc.ErrorMatches, "blammo")
}

func (s *FacadeSuite) TestWatchSuccess(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	backend := &mockBackend{exist: true, watch: true, entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, s.watcherRegistry, auth(true))
	c.Check(err, jc.ErrorIsNil)

	results, err := facade.Watch(context.Background(), modelEntity(s.modelUUID))
	c.Check(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}},
	})
}
