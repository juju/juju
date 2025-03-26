// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/go-goose/goose/v5/identity"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/version"
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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
