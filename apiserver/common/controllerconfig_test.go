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
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type controllerConfigSuite struct {
	testing.BaseSuite

	st                        *mocks.MockControllerConfigState
	controllerConfigService   *mocks.MockControllerConfigService
	controllerNodeService     *common.MockAPIAddressAccessor
	externalControllerService *mocks.MockExternalControllerService
	ctrlConfigAPI             *common.ControllerConfigAPI
}

func TestControllerConfigSuite(t *stdtesting.T) {
	tc.Run(t, &controllerConfigSuite{})
}

func (s *controllerConfigSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockControllerConfigState(ctrl)
	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.controllerNodeService = common.NewMockAPIAddressAccessor(ctrl)
	s.externalControllerService = mocks.NewMockExternalControllerService(ctrl)
	s.ctrlConfigAPI = common.NewControllerConfigAPI(s.st, s.controllerConfigService, s.controllerNodeService, s.externalControllerService)

	c.Cleanup(func() {
		s.st = nil
		s.controllerConfigService = nil
		s.controllerNodeService = nil
		s.externalControllerService = nil
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

	s.st.EXPECT().ModelExists(testing.ModelTag.Id()).Return(true, nil)
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
		s.localState,
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
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
		s.localState,
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
	)
	results, err := controllerConfig.ControllerAPIInfoForModels(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(modelUUID).String()}}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Addresses, tc.DeepEquals, info.Addrs)
	c.Assert(results.Results[0].CACert, tc.Equals, info.CACert)
}

func (s *controllerInfoSuite) TestControllerInfoMigratedController(c *tc.C) {
	domainServices := s.ControllerDomainServices(c)
	controllerConfig := common.NewControllerConfigAPI(
		s.localState,
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
	)

	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	model := s.localModel.State()

	targetControllerTag := names.NewControllerTag(uuid.MustNewUUID().String())
	defer model.Close()

	// Migrate the model and delete it from the state
	controllerIP := "1.2.3.4:5555"
	mig, err := model.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag:   targetControllerTag,
			ControllerAlias: "target",
			Addrs:           []string{controllerIP},
			CACert:          "",
			AuthTag:         names.NewUserTag("user2"),
			Password:        "secret",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig.SetPhase(phase), tc.ErrorIsNil)
	}

	c.Assert(s.localModel.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	c.Assert(model.RemoveDyingModel(), tc.ErrorIsNil)

	externalControllerInfo, err := controllerConfig.ControllerAPIInfoForModels(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(s.DefaultModelUUID.String()).String()}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(externalControllerInfo.Results), tc.Equals, 1)
	c.Assert(externalControllerInfo.Results[0].Addresses[0], tc.Equals, controllerIP)
}
