// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type controllerConfigSuite struct {
	testing.BaseSuite

	testingEnvConfig *config.Config
}

var _ = gc.Suite(&controllerConfigSuite{})

type fakeControllerAccessor struct {
	controllerConfigError error
}

func (f *fakeControllerAccessor) ControllerConfig() (controller.Config, error) {
	if f.controllerConfigError != nil {
		return nil, f.controllerConfigError
	}
	return map[string]interface{}{
		controller.ControllerUUIDKey: testing.ControllerTag.Id(),
		controller.CACertKey:         testing.CACert,
		controller.APIPort:           4321,
		controller.StatePort:         1234,
	}, nil
}

func (f *fakeControllerAccessor) ControllerInfo(modelUUID string) ([]string, string, error) {
	if modelUUID != testing.ModelTag.Id() {
		return nil, "", errors.New("wrong model")
	}
	return []string{"192.168.1.1:17070"}, testing.CACert, nil
}

func (s *controllerConfigSuite) TearDownTest(c *gc.C) {
	dummy.Reset(c)
	s.BaseSuite.TearDownTest(c)
}

func (*controllerConfigSuite) TestControllerConfigSuccess(c *gc.C) {
	cc := common.NewControllerConfig(
		&fakeControllerAccessor{},
	)
	result, err := cc.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(map[string]interface{}(result.Config), jc.DeepEquals, map[string]interface{}{
		"ca-cert":         testing.CACert,
		"controller-uuid": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		"state-port":      1234,
		"api-port":        4321,
	})
}

func (*controllerConfigSuite) TestControllerConfigFetchError(c *gc.C) {
	cc := common.NewControllerConfig(
		&fakeControllerAccessor{
			controllerConfigError: fmt.Errorf("pow"),
		},
	)
	_, err := cc.ControllerConfig()
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*controllerConfigSuite) TestControllerInfo(c *gc.C) {
	cc := common.NewControllerConfig(
		&fakeControllerAccessor{},
	)
	results, err := cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: testing.ModelTag.String()}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Addresses, gc.DeepEquals, []string{"192.168.1.1:17070"})
	c.Assert(results.Results[0].CACert, gc.Equals, testing.CACert)
}

type controllerInfoSuite struct {
	jujutesting.JujuConnSuite

	localState *state.State
	localModel *state.Model
}

var _ = gc.Suite(&controllerInfoSuite{})

func (s *controllerInfoSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.localState = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) {
		s.localState.Close()
	})
	model, err := s.localState.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.localModel = model
}

func (s *controllerInfoSuite) TestControllerInfoLocalModel(c *gc.C) {
	cc := common.NewStateControllerConfig(s.State)
	results, err := cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: s.localModel.ModelTag().String()}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	apiAddr, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Addresses, gc.HasLen, 1)
	c.Assert(results.Results[0].Addresses[0], gc.Equals, apiAddr[0][0].String())
	c.Assert(results.Results[0].CACert, gc.Equals, testing.CACert)
}

func (s *controllerInfoSuite) TestControllerInfoExternalModel(c *gc.C) {
	ec := state.NewExternalControllers(s.State)
	modelUUID := utils.MustNewUUID().String()
	info := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.1:12345"},
		CACert:        testing.CACert,
	}
	_, err := ec.Save(info, modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	cc := common.NewStateControllerConfig(s.State)
	results, err := cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(modelUUID).String()}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Addresses, gc.DeepEquals, info.Addrs)
	c.Assert(results.Results[0].CACert, gc.Equals, info.CACert)
}

func (s *controllerInfoSuite) TestControllerInfoMigratedController(c *gc.C) {
	cc := common.NewStateControllerConfig(s.State)
	modelState := s.Factory.MakeModel(c, &factory.ModelParams{})
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	targetControllerTag := names.NewControllerTag(utils.MustNewUUID().String())
	defer modelState.Close()

	// Migrate the model and delete it from the state
	controllerIP := "1.2.3.4:5555"
	mig, err := modelState.CreateMigration(state.MigrationSpec{
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
	c.Assert(err, jc.ErrorIsNil)
	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig.SetPhase(phase), jc.ErrorIsNil)
	}

	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(modelState.RemoveDyingModel(), jc.ErrorIsNil)

	externalControllerInfo, err := cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(model.UUID()).String()}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(externalControllerInfo.Results), gc.Equals, 1)
	c.Assert(externalControllerInfo.Results[0].Addresses[0], gc.Equals, controllerIP)
}
