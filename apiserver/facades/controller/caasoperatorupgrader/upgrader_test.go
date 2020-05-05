// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader_test

import (
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/caasoperatorupgrader"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	authorizer *apiservertesting.FakeAuthorizer
	api        *caasoperatorupgrader.API
	broker     *mockBroker
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.broker = &mockBroker{}
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("app"),
	}

	api, err := caasoperatorupgrader.NewCAASOperatorUpgraderAPI(s.authorizer, s.broker)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CAASProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasoperatorupgrader.NewCAASOperatorUpgraderAPI(s.authorizer, s.broker)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestUpgradeOperator(c *gc.C) {
	vers := version.MustParse("6.6.6")
	result, err := s.api.UpgradeOperator(params.KubernetesUpgradeArg{
		AgentTag: s.authorizer.Tag.String(),
		Version:  vers,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	s.broker.CheckCall(c, 0, "Upgrade", "app", vers)
}

func (s *CAASProvisionerSuite) assertUpgradeController(c *gc.C, tag names.Tag) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        tag,
		Controller: true,
	}

	api, err := caasoperatorupgrader.NewCAASOperatorUpgraderAPI(s.authorizer, s.broker)
	c.Assert(err, jc.ErrorIsNil)

	vers := version.MustParse("6.6.6")
	result, err := api.UpgradeOperator(params.KubernetesUpgradeArg{
		AgentTag: s.authorizer.Tag.String(),
		Version:  vers,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	s.broker.CheckCall(c, 0, "Upgrade", "controller", vers)
}

func (s *CAASProvisionerSuite) TestUpgradeLegacyController(c *gc.C) {
	s.assertUpgradeController(c, names.NewMachineTag("0"))
}

func (s *CAASProvisionerSuite) TestUpgradeController(c *gc.C) {
	s.assertUpgradeController(c, names.NewControllerAgentTag("0"))
}

type mockBroker struct {
	testing.Stub
}

func (m *mockBroker) Upgrade(app string, vers version.Number) error {
	m.AddCall("Upgrade", app, vers)
	return nil
}
