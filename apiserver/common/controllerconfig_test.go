// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type controllerConfigSuite struct {
	testing.BaseSuite

	st                        *mocks.MockControllerConfigState
	controllerConfigService   *mocks.MockControllerConfigService
	externalControllerService *mocks.MockExternalControllerService
	ctrlConfigAPI             *common.ControllerConfigAPI
}

var _ = gc.Suite(&controllerConfigSuite{})

func (s *controllerConfigSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockControllerConfigState(ctrl)
	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.externalControllerService = mocks.NewMockExternalControllerService(ctrl)
	s.ctrlConfigAPI = common.NewControllerConfigAPI(s.st, s.controllerConfigService, s.externalControllerService)
	return ctrl
}

func (s *controllerConfigSuite) TestControllerConfigSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(
		map[string]interface{}{
			controller.ControllerUUIDKey: testing.ControllerTag.Id(),
			controller.CACertKey:         testing.CACert,
			controller.APIPort:           4321,
			controller.StatePort:         1234,
		},
		nil,
	)

	result, err := s.ctrlConfigAPI.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(map[string]interface{}(result.Config), jc.DeepEquals, map[string]interface{}{
		"ca-cert":         testing.CACert,
		"controller-uuid": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		"state-port":      1234,
		"api-port":        4321,
	})
}

func (s *controllerConfigSuite) TestControllerConfigFetchError(c *gc.C) {
	defer s.setup(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(nil, fmt.Errorf("pow"))
	_, err := s.ctrlConfigAPI.ControllerConfig(context.Background())
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (s *controllerConfigSuite) expectStateControllerInfo(c *gc.C) {
	s.st.EXPECT().APIHostPortsForAgents(gomock.Any()).Return([]network.SpaceHostPorts{
		network.NewSpaceHostPorts(17070, "192.168.1.1"),
	}, nil)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(map[string]interface{}{
		controller.CACertKey: testing.CACert,
	}, nil)
}

func (s *controllerConfigSuite) TestControllerInfo(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().ModelExists(testing.ModelTag.Id()).Return(true, nil)
	s.expectStateControllerInfo(c)

	results, err := s.ctrlConfigAPI.ControllerAPIInfoForModels(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: testing.ModelTag.String()}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Addresses, gc.DeepEquals, []string{"192.168.1.1:17070"})
	c.Assert(results.Results[0].CACert, gc.Equals, testing.CACert)
}
