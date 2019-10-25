// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"time"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/spacenamer"
	"github.com/juju/juju/apiserver/facades/agent/spacenamer/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&spaceNamerAPISuite{})

type spaceNamerAPISuite struct {
	coretesting.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	state      *mocks.MockSpaceNamerState
	model      *mocks.MockModelCache
	resources  *facademocks.MockResources
}

func (s *spaceNamerAPISuite) TestSetDefaultSpaceName(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthController()
	s.expectAuthMachineAgent()
	s.expect(ctrl, "testme")

	facade := s.facadeAPI(c)
	result, err := facade.SetDefaultSpaceName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *spaceNamerAPISuite) TestSetDefaultSpaceNameCheckName(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthController()
	s.expectAuthMachineAgent()
	s.expect(ctrl, network.DefaultSpaceName)

	facade := s.facadeAPI(c)
	result, err := facade.SetDefaultSpaceName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *spaceNamerAPISuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.state = mocks.NewMockSpaceNamerState(ctrl)

	return ctrl
}

func (s *spaceNamerAPISuite) facadeAPI(c *gc.C) *spacenamer.SpaceNamerAPI {
	facade, err := spacenamer.NewSpaceNamerAPI(s.state, s.model, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return facade
}

func (s *spaceNamerAPISuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true).AnyTimes()
}

func (s *spaceNamerAPISuite) expectAuthMachineAgent() {
	s.authorizer.EXPECT().AuthMachineAgent().Return(true).AnyTimes()
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
	spaceNamerAPISuite

	watcher *mocks.MockNotifyWatcher

	notifyDone chan struct{}
}

func (s *spaceNamerAPIWatchSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.notifyDone = make(chan struct{})
}

func (s *spaceNamerAPIWatchSuite) TestWatchDefaultSpaceConfig(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectWatchConfigWithNotify(1)

	facade := s.facadeAPI(c)

	result, err := facade.WatchDefaultSpaceConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}},
	})
	s.assertNotifyStop(c)
}

func (s *spaceNamerAPIWatchSuite) TestWatchDefaultSpaceConfigWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectWatchConfigWithClosedChannel()

	facade := s.facadeAPI(c)

	_, err := facade.WatchDefaultSpaceConfig()
	c.Assert(err, gc.ErrorMatches, "cannot obtain")
}

func (s *spaceNamerAPIWatchSuite) TestWatchDefaultSpaceConfigModelCacheError(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectWatchConfigError()

	facade := s.facadeAPI(c)

	result, err := facade.WatchDefaultSpaceConfig()
	c.Assert(err, gc.ErrorMatches, "error from model cache")
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{})
}

func (s *spaceNamerAPIWatchSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.spaceNamerAPISuite.setup(c)

	s.model = mocks.NewMockModelCache(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)
	s.watcher = mocks.NewMockNotifyWatcher(ctrl)

	s.expectAuthMachineAgent()
	s.expectAuthController()

	return ctrl
}

func (s *spaceNamerAPIWatchSuite) expectWatchConfigWithNotify(times int) {
	ch := make(chan struct{})

	go func() {
		for i := 0; i < times; i++ {
			ch <- struct{}{}
		}
		close(s.notifyDone)
	}()

	s.model.EXPECT().WatchConfig(config.DefaultSpace).Return(s.watcher, nil)
	s.watcher.EXPECT().Changes().Return(ch)
	s.resources.EXPECT().Register(s.watcher).Return("1")
}

func (s *spaceNamerAPIWatchSuite) expectWatchConfigWithClosedChannel() {
	ch := make(chan struct{})
	close(ch)

	s.model.EXPECT().WatchConfig(config.DefaultSpace).Return(s.watcher, nil)
	s.watcher.EXPECT().Changes().Return(ch)
}

func (s *spaceNamerAPIWatchSuite) expectWatchConfigError() {
	s.model.EXPECT().WatchConfig(config.DefaultSpace, "").Return(s.watcher, errors.New("error from model cache"))
}

func (s *spaceNamerAPIWatchSuite) assertNotifyStop(c *gc.C) {
	select {
	case <-s.notifyDone:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
}
