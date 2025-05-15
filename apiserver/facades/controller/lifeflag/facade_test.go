// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/lifeflag"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testhelpers.IsolationSuite

	modelUUID       coremodel.UUID
	watcherRegistry *MockWatcherRegistry
}

var _ = tc.Suite(&FacadeSuite{})

func (s *FacadeSuite) SetUpTest(c *tc.C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
}

func (s *FacadeSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)
	return ctrl
}

func (s *FacadeSuite) TestFacadeAuthFailure(c *tc.C) {
	facade, err := lifeflag.NewFacade(s.modelUUID, nil, nil, auth(false))
	c.Check(facade, tc.IsNil)
	c.Check(err, tc.Equals, apiservererrors.ErrPerm)
}

func (s *FacadeSuite) TestLifeBadEntity(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Life(c.Context(), entities("archibald snookums"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, tc.Equals, life.Value(""))

	// TODO(fwereade): this is DUMB. should just be a parse error.
	// but I'm not fixing the underlying implementation as well.
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestLifeAuthFailure(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Life(c.Context(), entities("unit-foo-1"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, tc.Equals, life.Value(""))
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestLifeNotFound(c *tc.C) {
	backend := &mockBackend{entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Life(c.Context(), modelEntity(s.modelUUID))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.Life, tc.Equals, life.Value(""))
	c.Check(result.Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *FacadeSuite) TestLifeSuccess(c *tc.C) {
	backend := &mockBackend{exist: true, entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Check(err, tc.ErrorIsNil)

	results, err := facade.Life(c.Context(), modelEntity(s.modelUUID))
	c.Check(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{Life: life.Dying}},
	})
}

func (s *FacadeSuite) TestWatchBadEntity(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Watch(c.Context(), entities("archibald snookums"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, tc.Equals, "")

	// TODO(fwereade): this is DUMB. should just be a parse error.
	// but I'm not fixing the underlying implementation as well.
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestWatchAuthFailure(c *tc.C) {
	backend := &mockBackend{}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Watch(c.Context(), entities("unit-foo-1"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, tc.Equals, "")
	c.Check(result.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestWatchNotFound(c *tc.C) {
	backend := &mockBackend{exist: false, watch: true, entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, nil, auth(true))
	c.Assert(err, tc.ErrorIsNil)

	results, err := facade.Watch(c.Context(), modelEntity(s.modelUUID))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, tc.Equals, "")
	c.Check(result.Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *FacadeSuite) TestWatchBadWatcher(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	backend := &mockBackend{exist: true, watch: false, entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, s.watcherRegistry, auth(true))
	c.Check(err, tc.ErrorIsNil)

	results, err := facade.Watch(c.Context(), modelEntity(s.modelUUID))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Check(result.NotifyWatcherId, tc.Equals, "")
	c.Check(result.Error, tc.ErrorMatches, "blammo")
}

func (s *FacadeSuite) TestWatchSuccess(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	backend := &mockBackend{exist: true, watch: true, entity: names.NewModelTag(s.modelUUID.String())}
	facade, err := lifeflag.NewFacade(s.modelUUID, backend, s.watcherRegistry, auth(true))
	c.Check(err, tc.ErrorIsNil)

	results, err := facade.Watch(c.Context(), modelEntity(s.modelUUID))
	c.Check(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}},
	})
}
