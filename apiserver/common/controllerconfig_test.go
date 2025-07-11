// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type controllerConfigSuite struct {
	testing.BaseSuite

	controllerConfigService   *mocks.MockControllerConfigService
	controllerNodeService     *common.MockAPIAddressAccessor
	externalControllerService *mocks.MockExternalControllerService
	modelService              *mocks.MockModelService
	ctrlConfigAPI             *common.ControllerConfigAPI
}

func TestControllerConfigSuite(t *stdtesting.T) {
	tc.Run(t, &controllerConfigSuite{})
}

func (s *controllerConfigSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.controllerNodeService = common.NewMockAPIAddressAccessor(ctrl)
	s.externalControllerService = mocks.NewMockExternalControllerService(ctrl)
	s.modelService = mocks.NewMockModelService(ctrl)
	s.ctrlConfigAPI = common.NewControllerConfigAPI(s.controllerConfigService, s.controllerNodeService, s.externalControllerService, s.modelService)

	c.Cleanup(func() {
		s.controllerConfigService = nil
		s.controllerNodeService = nil
		s.externalControllerService = nil
		s.modelService = nil
		s.ctrlConfigAPI = nil
	})
	return ctrl
}

func (s *controllerConfigSuite) TestControllerConfigSuccess(c *tc.C) {
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

	result, err := s.ctrlConfigAPI.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(map[string]interface{}(result.Config), tc.DeepEquals, map[string]interface{}{
		"ca-cert":         testing.CACert,
		"controller-uuid": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		"state-port":      1234,
		"api-port":        4321,
	})
}

func (s *controllerConfigSuite) TestControllerConfigFetchError(c *tc.C) {
	defer s.setup(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(nil, fmt.Errorf("pow"))
	_, err := s.ctrlConfigAPI.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (s *controllerConfigSuite) expectControllerInfo() {
	addrs := []string{"192.168.1.1:17070"}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(map[string]interface{}{
		controller.CACertKey: testing.CACert,
	}, nil)
}

func (s *controllerConfigSuite) TestControllerInfo(c *tc.C) {
	defer s.setup(c).Finish()

	s.modelService.EXPECT().CheckModelExists(gomock.Any(), coremodel.UUID(testing.ModelTag.Id())).Return(true, nil)
	s.expectControllerInfo()

	results, err := s.ctrlConfigAPI.ControllerAPIInfoForModels(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: testing.ModelTag.String()}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Addresses, tc.DeepEquals, []string{"192.168.1.1:17070"})
	c.Assert(results.Results[0].CACert, tc.Equals, testing.CACert)
}

type controllerInfoSuite struct {
	jujutesting.ApiServerSuite

	localState *state.State
	localModel *state.Model
}

func TestControllerInfoSuite(t *stdtesting.T) {
	tc.Run(t, &controllerInfoSuite{})
}

func (s *controllerInfoSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.localState = f.MakeModel(c, &factory.ModelParams{
		UUID: s.DefaultModelUUID,
	})
	s.AddCleanup(func(*tc.C) {
		s.localState.Close()
	})
	model, err := s.localState.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.localModel = model
}

func (s *controllerInfoSuite) TestControllerInfoLocalModel(c *tc.C) {
	domainServices := s.ControllerDomainServices(c)
	controllerConfig := common.NewControllerConfigAPI(
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
		domainServices.Model(),
	)
	results, err := controllerConfig.ControllerAPIInfoForModels(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewModelTag(s.DefaultModelUUID.String()).String(),
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)

	// TODO: hml 18-06-2025
	// rewrite this piece when APIHostPortsForClients changes made.
	//systemState := s.ControllerModel(c).State()
	//apiAddr, err := systemState.APIHostPortsForClients(testing.FakeControllerConfig())
	//c.Assert(err, tc.ErrorIsNil)
	//c.Assert(results.Results[0].Addresses, tc.HasLen, 1)
	//c.Assert(results.Results[0].Addresses[0], tc.Equals, apiAddr[0][0].String())
	//c.Assert(results.Results[0].CACert, tc.Equals, testing.CACert)
}

func (s *controllerInfoSuite) TestControllerInfoExternalModel(c *tc.C) {
	modelUUID := uuid.MustNewUUID().String()
	info := crossmodel.ControllerInfo{
		ControllerUUID: testing.ControllerTag.Id(),
		Addrs:          []string{"192.168.1.1:12345"},
		CACert:         testing.CACert,
		ModelUUIDs:     []string{modelUUID},
	}
	domainServices := s.ControllerDomainServices(c)
	err := domainServices.ExternalController().UpdateExternalController(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)

	controllerConfig := common.NewControllerConfigAPI(
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
		domainServices.Model(),
	)
	results, err := controllerConfig.ControllerAPIInfoForModels(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(modelUUID).String()}}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Addresses, tc.DeepEquals, info.Addrs)
	c.Assert(results.Results[0].CACert, tc.Equals, info.CACert)
}

func (s *controllerInfoSuite) TestControllerInfoMigratedController(c *tc.C) {
	// TODO(modelmigration): test ControllerAPIInfoForModels when the model is
	// migrated to another controller.
	c.Skip("check ControllerAPIInfoForModels when the model is migrated elsewhere")
}
