// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/testing"
)

type environVpcSuite struct {
	BaseSuite
}

var _ = gc.Suite(&environVpcSuite{})

func (s *environVpcSuite) TestValidateBootstrapSubnetAutoCreate(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: ptr(true),
		SelfLink:              ptr("/path/to/network/some-vpc"),
	}, nil)
	s.MockService.EXPECT().NetworkFirewalls(gomock.Any(), "/path/to/network/some-vpc").
		Return([]*computepb.Firewall{{
			Allowed: []*computepb.Allowed{{
				IPProtocol: ptr("tcp"),
				Ports:      []string{"22"},
			}},
		}}, nil)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environVpcSuite) TestValidateBootstrapLegacyNetwork(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: nil,
		SelfLink:              ptr("/path/to/network/some-vpc"),
	}, nil)
	s.MockService.EXPECT().NetworkFirewalls(gomock.Any(), "/path/to/network/some-vpc").
		Return([]*computepb.Firewall{{
			Allowed: []*computepb.Allowed{{
				IPProtocol: ptr("tcp"),
				Ports:      []string{"22"},
			}},
		}}, nil)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environVpcSuite) TestValidateBootstrapNetworkNotFound(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(nil, errors.NotFound)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, jc.ErrorIs, errorVPCNotUsable)
}

func (s *environVpcSuite) TestValidateBootstrapUsableSubnets(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: ptr(false),
		SelfLink:              ptr("/path/to/network/some-vpc"),
		Subnetworks:           []string{"/path/to/subnet1", "/path/to/subnet2"},
	}, nil)
	s.MockService.EXPECT().NetworkFirewalls(gomock.Any(), "/path/to/network/some-vpc").
		Return([]*computepb.Firewall{{
			Allowed: []*computepb.Allowed{{
				IPProtocol: ptr("tcp"),
				Ports:      []string{"22"},
			}},
		}}, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1", "/path/to/subnet1", "/path/to/subnet2").
		Return([]*computepb.Subnetwork{{
			State: ptr("READY"),
		}, {
			State: ptr("DRAINING"),
		}}, nil)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environVpcSuite) TestValidateBootstrapNoSSHAccess(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: ptr(true),
		SelfLink:              ptr("/path/to/network/some-vpc"),
	}, nil)
	s.MockService.EXPECT().NetworkFirewalls(gomock.Any(), "/path/to/network/some-vpc").
		Return([]*computepb.Firewall{{
			Allowed: []*computepb.Allowed{{
				IPProtocol: ptr("tcp"),
				Ports:      []string{"80"},
			}},
		}}, nil)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, jc.ErrorIs, errorVPCNotUsable)
}

func (s *environVpcSuite) TestValidateBootstrapNoSubnets(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: ptr(false),
		SelfLink:              ptr("/path/to/network/some-vpc"),
	}, nil)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, jc.ErrorIs, errorVPCNotUsable)
}

func (s *environVpcSuite) TestValidateBootstrapNoUsableSubnets(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: ptr(false),
		SelfLink:              ptr("/path/to/network/some-vpc"),
		Subnetworks:           []string{"/path/to/subnet1", "/path/to/subnet2"},
	}, nil)
	s.MockService.EXPECT().NetworkFirewalls(gomock.Any(), "/path/to/network/some-vpc").
		Return([]*computepb.Firewall{{
			Allowed: []*computepb.Allowed{{
				IPProtocol: ptr("tcp"),
				Ports:      []string{"22"},
			}},
		}}, nil)
	s.MockService.EXPECT().Subnetworks(gomock.Any(), "us-east1", "/path/to/subnet1", "/path/to/subnet2").
		Return([]*computepb.Subnetwork{{
			State: ptr("DRAINING"),
		}, {
			State: ptr("DRAINING"),
		}}, nil)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, jc.ErrorIs, errorVPCNotRecommended)
}

func (s *environVpcSuite) TestValidateModelSubnet(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: ptr(true),
		SelfLink:              ptr("/path/to/network/some-vpc"),
	}, nil)

	err := validateModelVPC(s.CallCtx, s.MockService,
		"us-east1", "some-model", "some-vpc")
	c.Assert(err, jc.ErrorIsNil)
}
