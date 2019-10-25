// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer_test

import (
	"github.com/golang/mock/gomock"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/spacenamer"
	"github.com/juju/juju/apiserver/facades/agent/spacenamer/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/cache/cachetest"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

var _ = gc.Suite(&spaceNamerAPISuite{})

type spaceNamerAPISuite struct {
	coretesting.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	state      *mocks.MockSpaceNamerState
}

func (s *spaceNamerAPISuite) TestSetDefaultSpaceName(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthController(true)
	s.expect(ctrl, "testme")

	facade := s.facadeAPI(c)
	result, err := facade.SetDefaultSpaceName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *spaceNamerAPISuite) TestSetDefaultSpaceNameCheckName(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthController(true)
	s.expect(ctrl, network.DefaultSpaceName)

	facade := s.facadeAPI(c)
	result, err := facade.SetDefaultSpaceName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *spaceNamerAPISuite) TestSpaceNameAPINotController(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthController(false)

	facade, err := spacenamer.NewSpaceNamerAPI(s.state, nil, nil, s.authorizer)
	c.Assert(facade, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *spaceNamerAPISuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.state = mocks.NewMockSpaceNamerState(ctrl)

	return ctrl
}

func (s *spaceNamerAPISuite) facadeAPI(c *gc.C) *spacenamer.SpaceNamerAPI {
	facade, err := spacenamer.NewSpaceNamerAPI(s.state, nil, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return facade
}

func (s *spaceNamerAPISuite) expectAuthController(value bool) {
	s.authorizer.EXPECT().AuthController().Return(value).AnyTimes()
}

func (s *spaceNamerAPISuite) expect(ctrl *gomock.Controller, newName string) {
	space := mocks.NewMockSpace(ctrl)
	space.EXPECT().Name().Return(network.DefaultSpaceName)
	if newName != network.DefaultSpaceName {
		space.EXPECT().SetName(newName)
	}

	cfg := mocks.NewMockConfig(ctrl)
	cfg.EXPECT().DefaultSpace().Return(newName)

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().Config().Return(cfg, nil)

	sExp := s.state.EXPECT()
	sExp.Model().Return(model, nil)
	sExp.Space(network.DefaultSpaceId).Return(space, nil)
}

var _ = gc.Suite(&spaceNamerAPIWatchSuite{})

type spaceNamerAPIWatchSuite struct {
	statetesting.StateSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	spaceNamer *spacenamer.SpaceNamerAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	ctrl    *cachetest.TestController
	capture func(change interface{})
}

func (s *spaceNamerAPIWatchSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// The default auth is the controller
	s.authorizer = apiservertesting.FakeAuthorizer{
		Controller: true,
	}

	s.ctrl = cachetest.NewTestController(cachetest.ModelEvents)
	s.ctrl.Init(c)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, s.ctrl.Controller) })

	// Add the current model to the controller.
	m := cachetest.ModelChangeFromState(c, s.State)
	s.ctrl.SendChange(m)

	// Ensure it is processed before we create the logger API.
	_ = s.ctrl.NextChange(c)

	var err error
	s.spaceNamer, err = s.makeSpaceNamerAPI(s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceNamerAPIWatchSuite) TestWatchDefaultSpaceConfig(c *gc.C) {
	result, err := s.spaceNamer.WatchDefaultSpaceConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "1", Error: nil})

	resource := s.resources.Get(result.NotifyWatcherId)
	c.Assert(resource, gc.NotNil)

	_, ok := resource.(cache.NotifyWatcher)
	c.Assert(ok, jc.IsTrue)
}

func (s *spaceNamerAPIWatchSuite) makeSpaceNamerAPI(auth facade.Authorizer) (*spacenamer.SpaceNamerAPI, error) {
	ctx := facadetest.Context{
		Auth_:       auth,
		Controller_: s.ctrl.Controller,
		Resources_:  s.resources,
		State_:      s.State,
	}
	return spacenamer.NewFacade(ctx)
}
