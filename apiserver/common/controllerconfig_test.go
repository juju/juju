// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
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

	localModel *state.State
}

var _ = gc.Suite(&controllerInfoSuite{})

func (s *controllerInfoSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.localModel = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) {
		s.localModel.Close()
	})
}

func (s *controllerInfoSuite) TestControllerInfoLocalModel(c *gc.C) {
	cc := common.NewStateControllerConfig(s.State)
	results, err := cc.ControllerAPIInfoForModels(params.Entities{
		Entities: []params.Entity{{Tag: s.localModel.ModelTag().String()}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	apiAddr, err := s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Addresses, gc.HasLen, 1)
	c.Assert(results.Results[0].Addresses[0], gc.Equals, apiAddr[0][0].String())
	c.Assert(results.Results[0].CACert, gc.Equals, s.State.CACert())
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
