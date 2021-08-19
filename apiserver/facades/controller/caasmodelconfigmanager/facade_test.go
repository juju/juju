// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager"
	"github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager/mocks"
	"github.com/juju/juju/apiserver/params"
	jujucontroller "github.com/juju/juju/controller"
)

var _ = gc.Suite(&caasmodelconfigmanagerSuite{})

type caasmodelconfigmanagerSuite struct {
	testing.IsolationSuite

	backend         *mocks.MockBackend
	resources       *mocks.MockResources
	controllerState *mocks.MockControllerState
}

func (s *caasmodelconfigmanagerSuite) getFacade(c *gc.C) (*caasmodelconfigmanager.Facade, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.backend = mocks.NewMockBackend(ctrl)
	s.resources = mocks.NewMockResources(ctrl)
	s.controllerState = mocks.NewMockControllerState(ctrl)
	facade, err := caasmodelconfigmanager.NewFacadeForTest(
		s.backend, s.resources, s.controllerState,
	)
	c.Assert(err, jc.ErrorIsNil)
	return facade, ctrl
}

func (s *caasmodelconfigmanagerSuite) TestAuth(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := mocks.NewMockContext(ctrl)
	authorizer := mocks.NewMockAuthorizer(ctrl)

	gomock.InOrder(
		ctx.EXPECT().Auth().Return(authorizer),
		authorizer.EXPECT().AuthController().Return(false),
	)

	_, err := caasmodelconfigmanager.NewFacade(ctx)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *caasmodelconfigmanagerSuite) TestWatchControllerConfig(c *gc.C) {
	facade, ctrl := s.getFacade(c)
	defer ctrl.Finish()

	stateWatcher := mocks.NewMockNotifyWatcher(ctrl)
	watcherChan := make(chan struct{}, 1)
	watcherChan <- struct{}{}
	gomock.InOrder(
		s.backend.EXPECT().WatchControllerConfig().Return(stateWatcher),
		stateWatcher.EXPECT().Changes().Return(watcherChan),
		s.resources.EXPECT().Register(stateWatcher).Return(""),
	)

	watcher, err := facade.WatchControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)
}

func (s *caasmodelconfigmanagerSuite) TestControllerConfig(c *gc.C) {
	facade, ctrl := s.getFacade(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.controllerState.EXPECT().ControllerConfig().Return(
			jujucontroller.Config{
				"caas-image-repo": `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
			}, nil,
		),
	)

	cfg, err := facade.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		cfg.Config, gc.DeepEquals,
		params.ControllerConfig{
			"caas-image-repo": `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
		},
	)
}
