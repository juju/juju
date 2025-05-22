// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader_test

import (
	"context"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facades/controller/caasoperatorupgrader"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/semversion"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestCAASProvisionerSuite(t *testing.T) {
	tc.Run(t, &CAASProvisionerSuite{})
}

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	authorizer *apiservertesting.FakeAuthorizer
	api        *caasoperatorupgrader.API
	broker     *mockBroker
}

func (s *CAASProvisionerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.broker = &mockBroker{}
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("app"),
	}

	api, err := caasoperatorupgrader.NewCAASOperatorUpgraderAPI(s.authorizer, s.broker, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
}

func (s *CAASProvisionerSuite) TestPermission(c *tc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasoperatorupgrader.NewCAASOperatorUpgraderAPI(s.authorizer, s.broker, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestUpgradeOperator(c *tc.C) {
	vers := semversion.MustParse("6.6.6")
	result, err := s.api.UpgradeOperator(c.Context(), params.KubernetesUpgradeArg{
		AgentTag: s.authorizer.Tag.String(),
		Version:  vers,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	s.broker.CheckCall(c, 0, "Upgrade", s.authorizer.Tag.String(), vers)
}

func (s *CAASProvisionerSuite) assertUpgradeController(c *tc.C, tag names.Tag) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        tag,
		Controller: true,
	}

	api, err := caasoperatorupgrader.NewCAASOperatorUpgraderAPI(s.authorizer, s.broker, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	vers := semversion.MustParse("6.6.6")
	result, err := api.UpgradeOperator(c.Context(), params.KubernetesUpgradeArg{
		AgentTag: s.authorizer.Tag.String(),
		Version:  vers,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	s.broker.CheckCall(c, 0, "Upgrade", tag.String(), vers)
}

func (s *CAASProvisionerSuite) TestUpgradeLegacyController(c *tc.C) {
	s.assertUpgradeController(c, names.NewMachineTag("0"))
}

func (s *CAASProvisionerSuite) TestUpgradeController(c *tc.C) {
	s.assertUpgradeController(c, names.NewControllerAgentTag("0"))
}

type mockBroker struct {
	testhelpers.Stub
}

func (m *mockBroker) Upgrade(_ context.Context, app string, vers semversion.Number) error {
	m.AddCall("Upgrade", app, vers)
	return nil
}
