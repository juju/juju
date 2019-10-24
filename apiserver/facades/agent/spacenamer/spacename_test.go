// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/spacenamer"
	"github.com/juju/juju/apiserver/facades/agent/spacenamer/mocks"
)

var _ = gc.Suite(&spaceNamerAPISuite{})

type spaceNamerAPISuite struct {
	coretesting.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	state      *mocks.MockSpaceNamerState
	model      *mocks.MockModelCache
	resources  *facademocks.MockResources

	notifyDone chan struct{}
}

func (s *spaceNamerAPISuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.notifyDone = make(chan struct{})
}

//func (s *spaceNamerAPISuite) TestWatchDefaultSpaceConfig(c *gc.C) {
//
//}

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
	s.model = mocks.NewMockModelCache(ctrl)

	return ctrl
}

func (s *spaceNamerAPISuite) facadeAPI(c *gc.C) *spacenamer.SpaceNamerAPI {
	facade, err := spacenamer.NewSpaceNamerAPI(s.state, s.model, nil, s.authorizer)
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
	sExp.SpaceByID(network.DefaultSpaceId).Return(space, nil)
}
