// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
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
		controller.ControllerUUIDKey: testing.ModelTag.Id(),
		controller.CACertKey:         testing.CACert,
		controller.CAPrivateKey:      testing.CAKey,
		controller.ApiPort:           4321,
		controller.StatePort:         1234,
	}, nil
}

func (s *controllerConfigSuite) TearDownTest(c *gc.C) {
	dummy.Reset(c)
	s.BaseSuite.TearDownTest(c)
}

func (*controllerConfigSuite) TestControllerConfigSuccess(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	cc := common.NewControllerConfig(
		&fakeControllerAccessor{},
		nil,
		authorizer,
	)
	result, err := cc.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(map[string]interface{}(result.Config), jc.DeepEquals, map[string]interface{}{
		"ca-cert":         testing.CACert,
		"ca-private-key":  testing.CAKey,
		"controller-uuid": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"state-port":      1234,
		"api-port":        4321,
	})
}

func (*controllerConfigSuite) TestControllerConfigFetchError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	cc := common.NewControllerConfig(
		&fakeControllerAccessor{
			controllerConfigError: fmt.Errorf("pow"),
		},
		nil,
		authorizer,
	)
	_, err := cc.ControllerConfig()
	c.Assert(err, gc.ErrorMatches, "pow")
}
