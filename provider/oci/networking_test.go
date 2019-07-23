// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"

	"github.com/golang/mock/gomock"
	ociCore "github.com/oracle/oci-go-sdk/core"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

type networkingSuite struct {
	commonSuite
}

var _ = gc.Suite(&networkingSuite{})

func (n *networkingSuite) SetUpTest(c *gc.C) {
	n.commonSuite.SetUpTest(c)
}

func (n *networkingSuite) setupNetworkInterfacesExpectations(vnicID, vcnID string) {

	attachRequest, attachResponse := makeListVnicAttachmentsRequestResponse([]ociCore.VnicAttachment{
		{
			Id:                 makeStringPointer("fakeAttachmentId"),
			AvailabilityDomain: makeStringPointer("fake"),
			CompartmentId:      &n.testCompartment,
			InstanceId:         &n.testInstanceID,
			LifecycleState:     ociCore.VnicAttachmentLifecycleStateAttached,
			DisplayName:        makeStringPointer("fakeAttachmentName"),
			NicIndex:           makeIntPointer(0),
			VnicId:             &vnicID,
		},
	})

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

	vcnRequest, vcnResponse := makeListVcnRequestResponse([]ociCore.Vcn{
		{
			CompartmentId:         &n.testCompartment,
			CidrBlock:             makeStringPointer("1.0.0.0/8"),
			Id:                    makeStringPointer(vcnID),
			LifecycleState:        ociCore.VcnLifecycleStateAvailable,
			DefaultRouteTableId:   makeStringPointer("fakeRouteTable"),
			DefaultSecurityListId: makeStringPointer("fakeSeclist"),
			DisplayName:           makeStringPointer("amazingVcn"),
			FreeformTags:          n.tags,
		},
	})

	subnetRequest, subnetResponse := makeListSubnetsRequestResponse([]ociCore.Subnet{
		{
			AvailabilityDomain: makeStringPointer("us-phoenix-1"),
			CidrBlock:          makeStringPointer("1.0.0.0/8"),
			CompartmentId:      &n.testCompartment,
			Id:                 makeStringPointer("fakeSubnetId"),
			VcnId:              &vcnID,
			DisplayName:        makeStringPointer("fakeSubnet"),
			RouteTableId:       makeStringPointer("fakeRouteTable"),
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
		},
	})

	request, response := makeGetInstanceRequestResponse(ociCore.Instance{
		CompartmentId:      &n.testCompartment,
		AvailabilityDomain: makeStringPointer("QXay:PHX-AD-3"),
		Id:                 &n.testInstanceID,
		Region:             makeStringPointer("us-phoenix-1"),
		Shape:              makeStringPointer("VM.Standard1.1"),
		DisplayName:        makeStringPointer("fake"),
		FreeformTags:       n.tags,
		LifecycleState:     ociCore.InstanceLifecycleStateRunning,
	})

	gomock.InOrder(
		n.compute.EXPECT().GetInstance(context.Background(), request).Return(response, nil),
		n.compute.EXPECT().ListVnicAttachments(context.Background(), attachRequest).Return(attachResponse, nil),
		n.netw.EXPECT().GetVnic(context.Background(), vnicRequest[0]).Return(vnicResponse[0], nil),
		n.netw.EXPECT().ListVcns(context.Background(), vcnRequest).Return(vcnResponse, nil),
		n.netw.EXPECT().ListSubnets(context.Background(), subnetRequest).Return(subnetResponse, nil),
	)
}

func (n *networkingSuite) setupListSubnetsExpectations() {
	vcnID := "fakeVcn"

	vcnRequest, vcnResponse := makeListVcnRequestResponse(
		[]ociCore.Vcn{
			{
				CompartmentId:         &n.testCompartment,
				CidrBlock:             makeStringPointer("1.0.0.0/8"),
				Id:                    makeStringPointer(vcnID),
				LifecycleState:        ociCore.VcnLifecycleStateAvailable,
				DefaultRouteTableId:   makeStringPointer("fakeRouteTable"),
				DefaultSecurityListId: makeStringPointer("fakeSeclist"),
				DisplayName:           makeStringPointer("amazingVcn"),
				FreeformTags:          n.tags,
			},
		},
	)

	subnetRequest, subnetResponse := makeListSubnetsRequestResponse(
		[]ociCore.Subnet{
			{
				AvailabilityDomain: makeStringPointer("us-phoenix-1"),
				CidrBlock:          makeStringPointer("1.0.0.0/8"),
				CompartmentId:      &n.testCompartment,
				Id:                 makeStringPointer("fakeSubnetId"),
				VcnId:              makeStringPointer(vcnID),
				DisplayName:        makeStringPointer("fakeSubnet"),
				RouteTableId:       makeStringPointer("fakeRouteTable"),
				LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			},
		},
	)

	n.netw.EXPECT().ListVcns(context.Background(), vcnRequest).Return(vcnResponse, nil).Times(2)
	n.netw.EXPECT().ListSubnets(context.Background(), subnetRequest).Return(subnetResponse, nil).Times(2)
}

func (n *networkingSuite) setupSubnetsKnownInstanceExpectations() {
	vnicID := "fakeVnicId"
	vcnID := "fakeVcn"

	attachRequest, attachResponse := makeListVnicAttachmentsRequestResponse(
		[]ociCore.VnicAttachment{
			{
				Id:                 makeStringPointer("fakeAttachmentId"),
				AvailabilityDomain: makeStringPointer("fake"),
				CompartmentId:      &n.testCompartment,
				InstanceId:         &n.testInstanceID,
				LifecycleState:     ociCore.VnicAttachmentLifecycleStateAttached,
				DisplayName:        makeStringPointer("fakeAttachmentName"),
				NicIndex:           makeIntPointer(0),
				VnicId:             &vnicID,
			},
		},
	)

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

	vcnRequest, vcnResponse := makeListVcnRequestResponse(
		[]ociCore.Vcn{
			{
				CompartmentId:         &n.testCompartment,
				CidrBlock:             makeStringPointer("1.0.0.0/8"),
				Id:                    makeStringPointer(vcnID),
				LifecycleState:        ociCore.VcnLifecycleStateAvailable,
				DefaultRouteTableId:   makeStringPointer("fakeRouteTable"),
				DefaultSecurityListId: makeStringPointer("fakeSeclist"),
				DisplayName:           makeStringPointer("amazingVcn"),
				FreeformTags:          n.tags,
			},
		},
	)

	subnetRequest, subnetResponse := makeListSubnetsRequestResponse(
		[]ociCore.Subnet{
			{
				AvailabilityDomain: makeStringPointer("us-phoenix-1"),
				CidrBlock:          makeStringPointer("1.0.0.0/8"),
				CompartmentId:      &n.testCompartment,
				Id:                 makeStringPointer("fakeSubnetId"),
				VcnId:              &vcnID,
				DisplayName:        makeStringPointer("fakeSubnet"),
				RouteTableId:       makeStringPointer("fakeRouteTable"),
				LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			},
			{
				AvailabilityDomain: makeStringPointer("us-phoenix-1"),
				CidrBlock:          makeStringPointer("1.0.0.0/8"),
				CompartmentId:      &n.testCompartment,
				Id:                 makeStringPointer("anotherFakeSubnetId"),
				VcnId:              &vcnID,
				DisplayName:        makeStringPointer("fakeSubnet"),
				RouteTableId:       makeStringPointer("fakeRouteTable"),
				LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			},
		},
	)

	request, response := makeGetInstanceRequestResponse(
		ociCore.Instance{
			CompartmentId:      &n.testCompartment,
			AvailabilityDomain: makeStringPointer("QXay:PHX-AD-3"),
			Id:                 &n.testInstanceID,
			Region:             makeStringPointer("us-phoenix-1"),
			Shape:              makeStringPointer("VM.Standard1.1"),
			DisplayName:        makeStringPointer("fake"),
			FreeformTags:       n.tags,
			LifecycleState:     ociCore.InstanceLifecycleStateRunning,
		})

	n.netw.EXPECT().ListVcns(context.Background(), vcnRequest).Return(vcnResponse, nil).Times(2)
	n.netw.EXPECT().ListSubnets(context.Background(), subnetRequest).Return(subnetResponse, nil).Times(2)
	n.compute.EXPECT().GetInstance(context.Background(), request).Return(response, nil).Times(2)
	n.compute.EXPECT().ListVnicAttachments(context.Background(), attachRequest).Return(attachResponse, nil).Times(2)
	n.netw.EXPECT().GetVnic(context.Background(), vnicRequest[0]).Return(vnicResponse[0], nil).Times(2)
}

func (n *networkingSuite) TestNetworkInterfaces(c *gc.C) {
	ctrl := n.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	vcnID := "fakeVcn"

	n.setupNetworkInterfacesExpectations(vnicID, vcnID)

	info, err := n.env.NetworkInterfaces(nil, instance.Id(n.testInstanceID))
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0].Address, gc.Equals, network.NewScopedAddress("1.1.1.1", network.ScopeCloudLocal))
	c.Assert(info[0].InterfaceName, gc.Equals, "unsupported0")
	c.Assert(info[0].DeviceIndex, gc.Equals, 0)
	c.Assert(info[0].ProviderId, gc.Equals, corenetwork.Id(vnicID))
	c.Assert(info[0].MACAddress, gc.Equals, "aa:aa:aa:aa:aa:aa")
	c.Assert(info[0].InterfaceType, gc.Equals, network.EthernetInterface)
	c.Assert(info[0].ProviderSubnetId, gc.Equals, corenetwork.Id("fakeSubnetId"))
	c.Assert(info[0].CIDR, gc.Equals, "1.0.0.0/8")
}

func (n *networkingSuite) TestSubnets(c *gc.C) {
	ctrl := n.patchEnv(c)
	defer ctrl.Finish()

	n.setupListSubnetsExpectations()

	lookFor := []corenetwork.Id{
		corenetwork.Id("fakeSubnetId"),
	}
	info, err := n.env.Subnets(nil, instance.UnknownId, lookFor)
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0].CIDR, gc.Equals, "1.0.0.0/8")

	lookFor = []corenetwork.Id{
		corenetwork.Id("IDontExist"),
	}
	_, err = n.env.Subnets(nil, instance.UnknownId, lookFor)
	c.Check(err, gc.ErrorMatches, "failed to find the following subnet ids:.*IDontExist.*")
}

func (n *networkingSuite) TestSubnetsKnownInstanceId(c *gc.C) {
	ctrl := n.patchEnv(c)
	defer ctrl.Finish()

	n.setupSubnetsKnownInstanceExpectations()

	lookFor := []corenetwork.Id{
		corenetwork.Id("fakeSubnetId"),
	}
	info, err := n.env.Subnets(nil, instance.Id(n.testInstanceID), lookFor)
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0].CIDR, gc.Equals, "1.0.0.0/8")

	lookFor = []corenetwork.Id{
		corenetwork.Id("notHere"),
	}
	_, err = n.env.Subnets(nil, instance.Id(n.testInstanceID), lookFor)
	c.Check(err, gc.ErrorMatches, "failed to find the following subnet ids:.*notHere.*")
}
