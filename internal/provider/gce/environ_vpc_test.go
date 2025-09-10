// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	stdtesting "testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/environs/testing"
)

type environVpcSuite struct {
	BaseSuite
}

func TestEnvironSuite(t *stdtesting.T) {
	tc.Run(t, &environVpcSuite{})
}

func (s *environVpcSuite) TestValidateBootstrapSubnetAutoCreate(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environVpcSuite) TestValidateBootstrapLegacyNetwork(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environVpcSuite) TestValidateBootstrapNetworkNotFound(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(nil, errors.NotFound)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, tc.ErrorIs, errorVPCNotUsable)
}

func (s *environVpcSuite) TestValidateBootstrapUsableSubnets(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environVpcSuite) TestValidateBootstrapNoSSHAccess(c *tc.C) {
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
	c.Assert(err, tc.ErrorIs, errorVPCNotUsable)
}

func (s *environVpcSuite) TestValidateBootstrapNoSubnets(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: ptr(false),
		SelfLink:              ptr("/path/to/network/some-vpc"),
	}, nil)

	err := validateBootstrapVPC(
		testing.BootstrapContext(context.Background(), c), s.MockService,
		"us-east1", "some-vpc", false)
	c.Assert(err, tc.ErrorIs, errorVPCNotUsable)
}

func (s *environVpcSuite) TestValidateBootstrapNoUsableSubnets(c *tc.C) {
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
	c.Assert(err, tc.ErrorIs, errorVPCNotRecommended)
}

func (s *environVpcSuite) TestValidateModelSubnet(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Network(gomock.Any(), "some-vpc").Return(&computepb.Network{
		AutoCreateSubnetworks: ptr(true),
		SelfLink:              ptr("/path/to/network/some-vpc"),
	}, nil)

	err := validateModelVPC(c.Context(), s.MockService,
		"us-east1", "some-model", "some-vpc")
	c.Assert(err, tc.ErrorIsNil)
}
