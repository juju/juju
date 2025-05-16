// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
)

type networkingSuite struct {
	commonSuite
}

func TestNetworkingSuite(t *stdtesting.T) { tc.Run(t, &networkingSuite{}) }
func (s *networkingSuite) SetUpTest(c *tc.C) {
	s.commonSuite.SetUpTest(c)
}

func (s *networkingSuite) setupNetworkInterfacesExpectations(c *tc.C, vnicID, vcnID string) {
	attachResponse := []ociCore.VnicAttachment{
		{
			Id:                 makeStringPointer("fakeAttachmentId"),
			AvailabilityDomain: makeStringPointer("fake"),
			CompartmentId:      &s.testCompartment,
			InstanceId:         &s.testInstanceID,
			LifecycleState:     ociCore.VnicAttachmentLifecycleStateAttached,
			DisplayName:        makeStringPointer("fakeAttachmentName"),
			NicIndex:           makeIntPointer(0),
			VnicId:             &vnicID,
		},
	}

	vnicRequest, vnicResponse := makeGetVnicRequestResponse([]ociCore.GetVnicResponse{
		{
			Vnic: ociCore.Vnic{
				Id:             makeStringPointer(vnicID),
				PrivateIp:      makeStringPointer("1.1.1.1"),
				DisplayName:    makeStringPointer("fakeVnicName"),
				PublicIp:       makeStringPointer("2.2.2.2"),
				MacAddress:     makeStringPointer("aa:aa:aa:aa:aa:aa"),
				SubnetId:       makeStringPointer("fakeSubnetId"),
				LifecycleState: ociCore.VnicLifecycleStateAvailable,
			},
		},
	})

	vcnResponse := []ociCore.Vcn{
		{
			CompartmentId:         &s.testCompartment,
			CidrBlock:             makeStringPointer("1.0.0.0/8"),
			Id:                    makeStringPointer(vcnID),
			LifecycleState:        ociCore.VcnLifecycleStateAvailable,
			DefaultRouteTableId:   makeStringPointer("fakeRouteTable"),
			DefaultSecurityListId: makeStringPointer("fakeSeclist"),
			DisplayName:           makeStringPointer("amazingVcn"),
			FreeformTags:          s.tags,
		},
	}

	subnetResponse := []ociCore.Subnet{
		{
			AvailabilityDomain: makeStringPointer("us-phoenix-1"),
			CidrBlock:          makeStringPointer("1.0.0.0/8"),
			CompartmentId:      &s.testCompartment,
			Id:                 makeStringPointer("fakeSubnetId"),
			VcnId:              &vcnID,
			DisplayName:        makeStringPointer("fakeSubnet"),
			RouteTableId:       makeStringPointer("fakeRouteTable"),
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
		},
	}

	request, response := makeGetInstanceRequestResponse(ociCore.Instance{
		CompartmentId:      &s.testCompartment,
		AvailabilityDomain: makeStringPointer("QXay:PHX-AD-3"),
		Id:                 &s.testInstanceID,
		Region:             makeStringPointer("us-phoenix-1"),
		Shape:              makeStringPointer("VM.Standard1.1"),
		DisplayName:        makeStringPointer("fake"),
		FreeformTags:       s.tags,
		LifecycleState:     ociCore.InstanceLifecycleStateRunning,
	})

	gomock.InOrder(
		s.compute.EXPECT().GetInstance(gomock.Any(), request).Return(response, nil),
		s.compute.EXPECT().ListVnicAttachments(gomock.Any(), &s.testCompartment, &s.testInstanceID).Return(attachResponse, nil),
		s.netw.EXPECT().GetVnic(gomock.Any(), vnicRequest[0]).Return(vnicResponse[0], nil),
		s.netw.EXPECT().ListVcns(gomock.Any(), &s.testCompartment).Return(vcnResponse, nil),
		s.netw.EXPECT().ListSubnets(gomock.Any(), &s.testCompartment, &vcnID).Return(subnetResponse, nil),
	)
}

func (s *networkingSuite) setupListSubnetsExpectations(c *tc.C) {
	vcnID := "fakeVcn"

	vcnResponse := []ociCore.Vcn{
		{
			CompartmentId:         &s.testCompartment,
			CidrBlock:             makeStringPointer("1.0.0.0/8"),
			Id:                    makeStringPointer(vcnID),
			LifecycleState:        ociCore.VcnLifecycleStateAvailable,
			DefaultRouteTableId:   makeStringPointer("fakeRouteTable"),
			DefaultSecurityListId: makeStringPointer("fakeSeclist"),
			DisplayName:           makeStringPointer("amazingVcn"),
			FreeformTags:          s.tags,
		},
	}

	subnetResponse := []ociCore.Subnet{
		{
			AvailabilityDomain: makeStringPointer("us-phoenix-1"),
			CidrBlock:          makeStringPointer("1.0.0.0/8"),
			CompartmentId:      &s.testCompartment,
			Id:                 makeStringPointer("fakeSubnetId"),
			VcnId:              makeStringPointer(vcnID),
			DisplayName:        makeStringPointer("fakeSubnet"),
			RouteTableId:       makeStringPointer("fakeRouteTable"),
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
		},
	}

	s.netw.EXPECT().ListVcns(gomock.Any(), &s.testCompartment).Return(vcnResponse, nil).Times(2)
	s.netw.EXPECT().ListSubnets(gomock.Any(), &s.testCompartment, &vcnID).Return(subnetResponse, nil).Times(2)
}

func (s *networkingSuite) TestNetworkInterfaces(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	vcnID := "fakeVcn"

	s.setupNetworkInterfacesExpectations(c, vnicID, vcnID)

	infoList, err := s.env.NetworkInterfaces(c.Context(), []instance.Id{instance.Id(s.testInstanceID)})
	c.Assert(err, tc.IsNil)
	c.Assert(infoList, tc.HasLen, 1)
	info := infoList[0]

	c.Assert(info, tc.HasLen, 1)
	c.Assert(info[0].Addresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress(
			"1.1.1.1", network.WithScope(network.ScopeCloudLocal), network.WithCIDR("1.0.0.0/8"),
		).AsProviderAddress()})
	c.Assert(info[0].ShadowAddresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress("2.2.2.2", network.WithScope(network.ScopePublic)).AsProviderAddress()})
	c.Assert(info[0].DeviceIndex, tc.Equals, 0)
	c.Assert(info[0].ProviderId, tc.Equals, network.Id(vnicID))
	c.Assert(info[0].MACAddress, tc.Equals, "aa:aa:aa:aa:aa:aa")
	c.Assert(info[0].InterfaceType, tc.Equals, network.EthernetDevice)
	c.Assert(info[0].ProviderSubnetId, tc.Equals, network.Id("fakeSubnetId"))
}

func (s *networkingSuite) TestSubnets(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupListSubnetsExpectations(c)

	lookFor := []network.Id{
		network.Id("fakeSubnetId"),
	}
	info, err := s.env.Subnets(c.Context(), lookFor)
	c.Assert(err, tc.IsNil)
	c.Assert(info, tc.HasLen, 1)
	c.Assert(info[0].CIDR, tc.Equals, "1.0.0.0/8")

	lookFor = []network.Id{"IDontExist"}
	_, err = s.env.Subnets(c.Context(), lookFor)
	c.Check(err, tc.ErrorMatches, "failed to find the following subnet ids:.*IDontExist.*")
}
