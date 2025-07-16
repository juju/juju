// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"

	"github.com/go-goose/goose/v5/identity"
	"github.com/go-goose/goose/v5/neutron"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/context/mocks"
	"github.com/juju/juju/environs/tags"
)

type precheckUpgradesSuite struct {
	client *MockAuthenticatingClient
}

var _ = gc.Suite(&precheckUpgradesSuite{})

func (s *precheckUpgradesSuite) TestPrecheckUpgradeOperations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEndpointsHasNetwork()

	env := s.newEnvironForPrecheckUpgradeTest()
	ops := env.PrecheckUpgradeOperations()
	c.Assert(ops, gc.HasLen, 1)

	op := ops[0]
	c.Assert(op.TargetVersion, gc.Equals, version.MustParse("2.8.0"))
	c.Assert(op.Steps, gc.HasLen, 1)

	step := op.Steps[0]
	c.Assert(step.Description(), gc.Equals, "Verify Neutron OpenStack service enabled")
	c.Assert(step.Run(), jc.ErrorIsNil)
}

func (s *precheckUpgradesSuite) TestPrecheckUpgradeOperationsFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectEndpointsNoNetwork()

	env := s.newEnvironForPrecheckUpgradeTest()
	ops := env.PrecheckUpgradeOperations()
	c.Assert(ops, gc.HasLen, 1)

	op := ops[0]
	c.Assert(op.Steps, gc.HasLen, 1)

	err := op.Steps[0].Run()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *precheckUpgradesSuite) newEnvironForPrecheckUpgradeTest() *Environ {
	return &Environ{
		clientUnlocked: s.client,
		cloudUnlocked: environscloudspec.CloudSpec{
			Region: "Region",
		},
	}
}

func (s *precheckUpgradesSuite) expectEndpointsHasNetwork() {
	endPts := identity.ServiceURLs{
		"network": "testing",
	}
	exp := s.client.EXPECT()
	exp.EndpointsForRegion(gomock.Any()).Return(endPts)
}

func (s *precheckUpgradesSuite) expectEndpointsNoNetwork() {
	endPts := identity.ServiceURLs{
		"compute": "testing",
	}
	exp := s.client.EXPECT()
	exp.EndpointsForRegion(gomock.Any()).Return(endPts)
}

func (s *precheckUpgradesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = NewMockAuthenticatingClient(ctrl)
	return ctrl
}

type upgraderSuite struct {
	neutronClient *MockNetworkingNeutron
	ctx           context.ProviderCallContext
}

var _ = gc.Suite(&upgraderSuite{})

func (s *upgraderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.neutronClient = NewMockNetworkingNeutron(ctrl)
	s.ctx = mocks.NewMockProviderCallContext(ctrl)
	return ctrl
}

func (s *upgraderSuite) newEnvironForUpgradeStepTest() *Environ {
	return &Environ{
		neutronUnlocked: s.neutronClient,
		controllerUUID:  utils.MustNewUUID().String(),
		modelUUID:       utils.MustNewUUID().String(),
	}
}

func (s *upgraderSuite) TestUpgradeOperations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	env := s.newEnvironForUpgradeStepTest()

	ops := env.UpgradeOperations(s.ctx, environs.UpgradeOperationsParams{ControllerUUID: "dummy-uuid"})

	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].TargetVersion, gc.Equals, 1)
	c.Assert(ops[0].Steps, gc.HasLen, 1)
	c.Assert(ops[0].Steps[0].Description(), gc.Equals, "Add tags to existing security groups")
}

func (s *upgraderSuite) TestDescription(c *gc.C) {
	defer s.setupMocks(c).Finish()
	env := s.newEnvironForUpgradeStepTest()
	tagGroupStep := tagExistingSecurityGroupsStep{env}

	desc := tagGroupStep.Description()

	c.Assert(desc, gc.Equals, "Add tags to existing security groups")
}

func (s *upgraderSuite) TestRun(c *gc.C) {
	defer s.setupMocks(c).Finish()
	env := s.newEnvironForUpgradeStepTest()
	tagGroupStep := tagExistingSecurityGroupsStep{env}
	controllerUuid := env.controllerUUID
	modelUuid := env.modelUUID
	securityGroups := []neutron.SecurityGroupV2{
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "juju-" + controllerUuid + "-" + modelUuid,
			Description: "juju group",
			Tags:        []string{},
		},
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "juju-" + controllerUuid + "-" + modelUuid + "-0",
			Description: "juju group",
			Tags:        []string{},
		},
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "juju-" + controllerUuid + "-" + modelUuid,
			Description: "juju group",
			Tags:        []string{},
		},
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "juju-" + controllerUuid + "-" + modelUuid + "-0",
			Description: "juju group",
			Tags:        []string{},
		},
	}
	s.neutronClient.EXPECT().ListSecurityGroupsV2(neutron.ListSecurityGroupsV2Query{}).Return(securityGroups, nil)
	gomock.InOrder(
		s.neutronClient.EXPECT().ReplaceAllTags("security-groups", securityGroups[0].Id, []string{
			fmt.Sprintf("%s=%s", tags.JujuController, controllerUuid),
			fmt.Sprintf("%s=%s", tags.JujuModel, modelUuid)}),
		s.neutronClient.EXPECT().ReplaceAllTags("security-groups", securityGroups[1].Id, []string{
			fmt.Sprintf("%s=%s", tags.JujuController, controllerUuid),
			fmt.Sprintf("%s=%s", tags.JujuModel, modelUuid)}),
		s.neutronClient.EXPECT().ReplaceAllTags("security-groups", securityGroups[2].Id, []string{
			fmt.Sprintf("%s=%s", tags.JujuController, controllerUuid),
			fmt.Sprintf("%s=%s", tags.JujuModel, modelUuid)}),
		s.neutronClient.EXPECT().ReplaceAllTags("security-groups", securityGroups[3].Id, []string{
			fmt.Sprintf("%s=%s", tags.JujuController, controllerUuid),
			fmt.Sprintf("%s=%s", tags.JujuModel, modelUuid)}),
	)

	err := tagGroupStep.Run(s.ctx)

	c.Assert(err, gc.IsNil)
}

func (s *upgraderSuite) TestRunSkipGroupsInDifferentModel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	env := s.newEnvironForUpgradeStepTest()
	tagGroupStep := tagExistingSecurityGroupsStep{env}
	controllerUUID := env.controllerUUID
	modelUUID := env.modelUUID
	otherModelUuid := utils.MustNewUUID().String()
	securityGroups := []neutron.SecurityGroupV2{
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "juju-" + controllerUUID + "-" + modelUUID,
			Description: "juju group",
			Tags:        []string{},
		},
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "juju-" + controllerUUID + "-" + modelUUID + "-0",
			Description: "juju group",
			Tags:        []string{},
		},
		// should not tag this group
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "Default",
			Description: "not a juju group",
			Tags:        []string{},
		},
		// should not tag this group
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "juju-" + controllerUUID + "-" + otherModelUuid,
			Description: "juju group",
			Tags:        []string{},
		},
		// should not tag this group
		{
			Id:          utils.MustNewUUID().String(),
			Name:        "Some Testing Group",
			Description: "not a juju group",
			Tags:        []string{},
		},
	}
	s.neutronClient.EXPECT().ListSecurityGroupsV2(neutron.ListSecurityGroupsV2Query{}).Return(securityGroups, nil)
	gomock.InOrder(
		s.neutronClient.EXPECT().ReplaceAllTags("security-groups", securityGroups[0].Id, []string{
			fmt.Sprintf("%s=%s", tags.JujuController, controllerUUID),
			fmt.Sprintf("%s=%s", tags.JujuModel, modelUUID)}),
		s.neutronClient.EXPECT().ReplaceAllTags("security-groups", securityGroups[1].Id, []string{
			fmt.Sprintf("%s=%s", tags.JujuController, controllerUUID),
			fmt.Sprintf("%s=%s", tags.JujuModel, modelUUID)}),
	)

	err := tagGroupStep.Run(s.ctx)

	c.Assert(err, gc.IsNil)
}
