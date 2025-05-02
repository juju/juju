// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"
	"strings"

	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/common/mocks"
	"github.com/juju/juju/internal/provider/oci"
)

type instanceSuite struct {
	commonSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)
}

func (s *instanceSuite) TestNewInstance(c *gc.C) {
	_, err := oci.NewInstance(ociCore.Instance{}, s.env)
	c.Assert(err, gc.ErrorMatches, "Instance response does not contain an ID")
}

func (s *instanceSuite) TestId(c *gc.C) {
	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, gc.IsNil)
	id := inst.Id()
	c.Assert(id, gc.Equals, instance.Id(s.testInstanceID))
}

func (s *instanceSuite) TestStatus(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().GetInstance(gomock.Any(), gomock.Any()).Return(ociCore.GetInstanceResponse{Instance: *s.ociInstance}, nil)
	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, gc.IsNil)

	instStatus := inst.Status(context.Background())
	expectedStatus := instance.Status{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateRunning)),
	}
	c.Assert(instStatus, gc.DeepEquals, expectedStatus)

	// Change lifecycle and check again
	s.ociInstance.LifecycleState = ociCore.InstanceLifecycleStateTerminating

	s.compute.EXPECT().GetInstance(gomock.Any(), gomock.Any()).Return(ociCore.GetInstanceResponse{Instance: *s.ociInstance}, nil)
	inst, err = oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, gc.IsNil)

	instStatus = inst.Status(context.Background())
	expectedStatus = instance.Status{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateTerminating)),
	}
	c.Assert(instStatus, gc.DeepEquals, expectedStatus)
}

func (s *instanceSuite) TestStatusNilRawInstanceResponse(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	request, response := makeGetInstanceRequestResponse(
		ociCore.Instance{
			CompartmentId:      &s.testCompartment,
			AvailabilityDomain: makeStringPointer("QXay:PHX-AD-3"),
			Id:                 &s.testInstanceID,
			Region:             makeStringPointer("us-phoenix-1"),
			Shape:              makeStringPointer("VM.Standard1.1"),
			DisplayName:        makeStringPointer("fake"),
			FreeformTags:       s.tags,
			LifecycleState:     ociCore.InstanceLifecycleStateRunning,
		})

	s.compute.EXPECT().GetInstance(context.Background(), request).Return(response, nil)

	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, gc.IsNil)

	instStatus := inst.Status(context.Background())
	expectedStatus := instance.Status{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateRunning)),
	}
	c.Assert(instStatus, gc.DeepEquals, expectedStatus)
}

func (s *instanceSuite) setupListVnicsExpectations(instanceId, vnicID string) {
	attachResponse := []ociCore.VnicAttachment{
		{
			Id:                 makeStringPointer("fakeAttachmentId"),
			AvailabilityDomain: makeStringPointer("fake"),
			CompartmentId:      makeStringPointer(s.testCompartment),
			InstanceId:         makeStringPointer(s.testInstanceID),
			LifecycleState:     ociCore.VnicAttachmentLifecycleStateAttached,
			DisplayName:        makeStringPointer("fakeAttachmentName"),
			NicIndex:           makeIntPointer(0),
			VnicId:             makeStringPointer(vnicID),
		},
	}

	// I am really sorry for this
	trueBoolean := true

	vnicRequest, vnicResponse := makeGetVnicRequestResponse([]ociCore.GetVnicResponse{
		{
			Vnic: ociCore.Vnic{
				Id:             makeStringPointer(vnicID),
				PrivateIp:      makeStringPointer("1.1.1.1"),
				SubnetId:       makeStringPointer("fakeSubnetId"),
				DisplayName:    makeStringPointer("fakeVnicName"),
				MacAddress:     makeStringPointer("11:11:11:11:11:11"),
				PublicIp:       makeStringPointer("2.2.2.2"),
				IsPrimary:      &trueBoolean,
				LifecycleState: ociCore.VnicLifecycleStateAvailable,
			},
		},
	})

	gomock.InOrder(
		s.compute.EXPECT().ListVnicAttachments(context.Background(), &s.testCompartment, &s.testInstanceID).Return(attachResponse, nil),
		s.netw.EXPECT().GetVnic(context.Background(), vnicRequest[0]).Return(vnicResponse[0], nil),
	)
}

func (s *instanceSuite) TestAddresses(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	s.setupListVnicsExpectations(s.testInstanceID, vnicID)

	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, gc.IsNil)

	addresses, err := inst.Addresses(context.Background())
	c.Assert(err, gc.IsNil)
	c.Check(addresses, gc.HasLen, 2)
	c.Check(addresses[0].Scope, gc.Equals, corenetwork.ScopeCloudLocal)
	c.Check(addresses[1].Scope, gc.Equals, corenetwork.ScopePublic)
}

func (s *instanceSuite) TestAddressesNoPublicIP(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	s.setupListVnicsExpectations(s.testInstanceID, vnicID)

	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, gc.IsNil)

	addresses, err := inst.Addresses(context.Background())
	c.Assert(err, gc.IsNil)
	c.Check(addresses, gc.HasLen, 2)
	c.Check(addresses[0].Scope, gc.Equals, corenetwork.ScopeCloudLocal)
	c.Check(addresses[1].Scope, gc.Equals, corenetwork.ScopePublic)
}

func (s *instanceSuite) TestInstanceConfiguratorUsesPublicAddress(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	s.setupListVnicsExpectations(s.testInstanceID, vnicID)

	rules := firewall.IngressRules{{
		PortRange: corenetwork.PortRange{
			FromPort: 1234,
			ToPort:   1234,
			Protocol: "tcp",
		},
	}}

	ic := mocks.NewMockInstanceConfigurator(ctrl)
	ic.EXPECT().ChangeIngressRules("", true, rules).Return(nil)

	factory := func(addr string) common.InstanceConfigurator {
		c.Assert(addr, gc.Equals, "2.2.2.2")
		return ic
	}

	inst, err := oci.NewInstanceWithConfigurator(*s.ociInstance, s.env, factory)
	c.Assert(err, gc.IsNil)
	c.Assert(inst.OpenPorts(context.Background(), "", rules), gc.IsNil)
}
