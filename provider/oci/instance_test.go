// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"
	"strings"

	gomock "github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	ociCore "github.com/oracle/oci-go-sdk/core"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/oci"
)

type instanceSuite struct {
	commonSuite
}

var _ = gc.Suite(&instanceSuite{})

func (i *instanceSuite) SetUpTest(c *gc.C) {
	i.commonSuite.SetUpTest(c)
}

func (i *instanceSuite) TestNewInstance(c *gc.C) {
	_, err := oci.NewInstance(ociCore.Instance{}, i.env)
	c.Assert(err, gc.ErrorMatches, "Instance response does not contain an ID")
}

func (i *instanceSuite) TestId(c *gc.C) {
	inst, err := oci.NewInstance(*i.ociInstance, i.env)
	c.Assert(err, gc.IsNil)
	id := inst.Id()
	c.Assert(id, gc.Equals, instance.Id(i.testInstanceID))
}

func (i *instanceSuite) TestStatus(c *gc.C) {
	ctrl := i.patchEnv(c)
	defer ctrl.Finish()

	i.compute.EXPECT().GetInstance(gomock.Any(), gomock.Any()).Return(ociCore.GetInstanceResponse{Instance: *i.ociInstance}, nil)
	inst, err := oci.NewInstance(*i.ociInstance, i.env)
	c.Assert(err, gc.IsNil)

	instStatus := inst.Status(nil)
	expectedStatus := instance.InstanceStatus{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateRunning)),
	}
	c.Assert(instStatus, gc.DeepEquals, expectedStatus)

	// Change lifecycle and check again
	i.ociInstance.LifecycleState = ociCore.InstanceLifecycleStateTerminating

	i.compute.EXPECT().GetInstance(gomock.Any(), gomock.Any()).Return(ociCore.GetInstanceResponse{Instance: *i.ociInstance}, nil)
	inst, err = oci.NewInstance(*i.ociInstance, i.env)
	c.Assert(err, gc.IsNil)

	instStatus = inst.Status(nil)
	expectedStatus = instance.InstanceStatus{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateTerminating)),
	}
	c.Assert(instStatus, gc.DeepEquals, expectedStatus)
}

func (i *instanceSuite) TestStatusNilRawInstanceResponse(c *gc.C) {
	ctrl := i.patchEnv(c)
	defer ctrl.Finish()

	request, response := makeGetInstanceRequestResponse(
		ociCore.Instance{
			CompartmentId:      &i.testCompartment,
			AvailabilityDomain: makeStringPointer("QXay:PHX-AD-3"),
			Id:                 &i.testInstanceID,
			Region:             makeStringPointer("us-phoenix-1"),
			Shape:              makeStringPointer("VM.Standard1.1"),
			DisplayName:        makeStringPointer("fake"),
			FreeformTags:       i.tags,
			LifecycleState:     ociCore.InstanceLifecycleStateRunning,
		})

	i.compute.EXPECT().GetInstance(context.Background(), request).Return(response, nil)

	inst, err := oci.NewInstance(*i.ociInstance, i.env)
	c.Assert(err, gc.IsNil)

	instStatus := inst.Status(nil)
	expectedStatus := instance.InstanceStatus{
		Status:  status.Running,
		Message: strings.ToLower(string(ociCore.InstanceLifecycleStateRunning)),
	}
	c.Assert(instStatus, gc.DeepEquals, expectedStatus)
}

func (i *instanceSuite) setupListVnicsExpectations(instanceId, vnicID string) {
	attachRequest, attachResponse := makeListVnicAttachmentsRequestResponse(
		[]ociCore.VnicAttachment{
			{
				Id:                 makeStringPointer("fakeAttachmentId"),
				AvailabilityDomain: makeStringPointer("fake"),
				CompartmentId:      makeStringPointer(i.testCompartment),
				InstanceId:         makeStringPointer(i.testInstanceID),
				LifecycleState:     ociCore.VnicAttachmentLifecycleStateAttached,
				DisplayName:        makeStringPointer("fakeAttachmentName"),
				NicIndex:           makeIntPointer(0),
				VnicId:             makeStringPointer(vnicID),
			},
		},
	)

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
		i.compute.EXPECT().ListVnicAttachments(context.Background(), attachRequest).Return(attachResponse, nil),
		i.netw.EXPECT().GetVnic(context.Background(), vnicRequest[0]).Return(vnicResponse[0], nil),
	)
}

func (i *instanceSuite) TestAddresses(c *gc.C) {
	ctrl := i.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	i.setupListVnicsExpectations(i.testInstanceID, vnicID)

	inst, err := oci.NewInstance(*i.ociInstance, i.env)
	c.Assert(err, gc.IsNil)

	addresses, err := inst.Addresses(nil)
	c.Assert(err, gc.IsNil)
	c.Check(addresses, gc.HasLen, 2)
	c.Check(addresses[0].Scope, gc.Equals, network.ScopeCloudLocal)
	c.Check(addresses[1].Scope, gc.Equals, network.ScopePublic)
}

func (i *instanceSuite) TestAddressesNoPublicIP(c *gc.C) {
	ctrl := i.patchEnv(c)
	defer ctrl.Finish()

	vnicID := "fakeVnicId"
	i.setupListVnicsExpectations(i.testInstanceID, vnicID)

	inst, err := oci.NewInstance(*i.ociInstance, i.env)
	c.Assert(err, gc.IsNil)

	addresses, err := inst.Addresses(nil)
	c.Assert(err, gc.IsNil)
	c.Check(addresses, gc.HasLen, 2)
	c.Check(addresses[0].Scope, gc.Equals, network.ScopeCloudLocal)
	c.Check(addresses[1].Scope, gc.Equals, network.ScopePublic)
}
