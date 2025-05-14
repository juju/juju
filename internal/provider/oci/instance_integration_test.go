// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"strings"

	"github.com/juju/tc"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/mock/gomock"

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

var _ = tc.Suite(&instanceSuite{})

func (s *instanceSuite) SetUpTest(c *tc.C) {
	s.commonSuite.SetUpTest(c)
}

func (s *instanceSuite) TestNewInstance(c *tc.C) {
	_, err := oci.NewInstance(ociCore.Instance{}, s.env)
	c.Assert(err, tc.ErrorMatches, "Instance response does not contain an ID")
}

func (s *instanceSuite) TestId(c *tc.C) {
	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, tc.IsNil)
	id := inst.Id()
	c.Assert(id, tc.Equals, instance.Id(s.testInstanceID))
}

func (s *instanceSuite) TestStatus(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().GetInstance(gomock.Any(), gomock.Any()).Return(ociCore.GetInstanceResponse{Instance: *s.ociInstance}, nil)
	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, tc.IsNil)

	instStatus := inst.Status(c.Context())
	expectedStatus := instance.Status{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateRunning)),
	}
	c.Assert(instStatus, tc.DeepEquals, expectedStatus)

	// Change lifecycle and check again
	s.ociInstance.LifecycleState = ociCore.InstanceLifecycleStateTerminating

	s.compute.EXPECT().GetInstance(gomock.Any(), gomock.Any()).Return(ociCore.GetInstanceResponse{Instance: *s.ociInstance}, nil)
	inst, err = oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, tc.IsNil)

	instStatus = inst.Status(c.Context())
	expectedStatus = instance.Status{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateTerminating)),
	}
	c.Assert(instStatus, tc.DeepEquals, expectedStatus)
}

func (s *instanceSuite) TestStatusNilRawInstanceResponse(c *tc.C) {
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

	s.compute.EXPECT().GetInstance(gomock.Any(), request).Return(response, nil)

	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, tc.IsNil)

	instStatus := inst.Status(c.Context())
	expectedStatus := instance.Status{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateRunning)),
	}
	c.Assert(instStatus, tc.DeepEquals, expectedStatus)
}

func (s *instanceSuite) setupListVnicsExpectations(c *tc.C, instanceId, vnicID string) {
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
		s.compute.EXPECT().ListVnicAttachments(gomock.Any(), &s.testCompartment, &s.testInstanceID).Return(attachResponse, nil),
		s.netw.EXPECT().GetVnic(gomock.Any(), vnicRequest[0]).Return(vnicResponse[0], nil),
	)
}

func (s *instanceSuite) TestAddresses(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	s.setupListVnicsExpectations(c, s.testInstanceID, vnicID)

	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, tc.IsNil)

	addresses, err := inst.Addresses(c.Context())
	c.Assert(err, tc.IsNil)
	c.Check(addresses, tc.HasLen, 2)
	c.Check(addresses[0].Scope, tc.Equals, corenetwork.ScopeCloudLocal)
	c.Check(addresses[1].Scope, tc.Equals, corenetwork.ScopePublic)
}

func (s *instanceSuite) TestAddressesNoPublicIP(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	s.setupListVnicsExpectations(c, s.testInstanceID, vnicID)

	inst, err := oci.NewInstance(*s.ociInstance, s.env)
	c.Assert(err, tc.IsNil)

	addresses, err := inst.Addresses(c.Context())
	c.Assert(err, tc.IsNil)
	c.Check(addresses, tc.HasLen, 2)
	c.Check(addresses[0].Scope, tc.Equals, corenetwork.ScopeCloudLocal)
	c.Check(addresses[1].Scope, tc.Equals, corenetwork.ScopePublic)
}

func (s *instanceSuite) TestInstanceConfiguratorUsesPublicAddress(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	s.setupListVnicsExpectations(c, s.testInstanceID, vnicID)

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
		c.Assert(addr, tc.Equals, "2.2.2.2")
		return ic
	}

	inst, err := oci.NewInstanceWithConfigurator(*s.ociInstance, s.env, factory)
	c.Assert(err, tc.IsNil)
	c.Assert(inst.OpenPorts(c.Context(), "", rules), tc.IsNil)
}
