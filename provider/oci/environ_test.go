// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"
	"fmt"
	"net/http"

	gomock "github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	ociCore "github.com/oracle/oci-go-sdk/core"
	ociIdentity "github.com/oracle/oci-go-sdk/identity"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/oci"
	"github.com/juju/juju/testing"
)

type environSuite struct {
	commonSuite

	listInstancesRequest  ociCore.ListInstancesRequest
	listInstancesResponse ociCore.ListInstancesResponse
}

var _ = gc.Suite(&environSuite{})

func (e *environSuite) SetUpTest(c *gc.C) {
	e.commonSuite.SetUpTest(c)
	*oci.MaxPollIterations = 2
	e.listInstancesRequest, e.listInstancesResponse = makeListInstancesRequestResponse(
		[]ociCore.Instance{
			{
				AvailabilityDomain: makeStringPointer("fakeZone1"),
				CompartmentId:      &e.testCompartment,
				Id:                 makeStringPointer("fakeInstance1"),
				LifecycleState:     ociCore.InstanceLifecycleStateRunning,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName"),
				FreeformTags:       e.tags,
			},
			{
				AvailabilityDomain: makeStringPointer("fakeZone2"),
				CompartmentId:      &e.testCompartment,
				Id:                 makeStringPointer("fakeInstance2"),
				LifecycleState:     ociCore.InstanceLifecycleStateRunning,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName2"),
				FreeformTags:       e.tags,
			},
		},
	)
}

func (e *environSuite) setupAvailabilityDomainsExpectations(times int) {
	request, response := makeListAvailabilityDomainsRequestResponse([]ociIdentity.AvailabilityDomain{
		{
			Name:          makeStringPointer("fakeZone1"),
			CompartmentId: &e.testCompartment,
		},
		{
			Name:          makeStringPointer("fakeZone2"),
			CompartmentId: &e.testCompartment,
		},
		{
			Name:          makeStringPointer("fakeZone3"),
			CompartmentId: &e.testCompartment,
		},
	})

	expect := e.ident.EXPECT().ListAvailabilityDomains(context.Background(), request).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func makeVcnName(controllerUUID, modelUUID string) string {
	return fmt.Sprintf("%s-%s-%s", oci.VcnNamePrefix, controllerUUID, modelUUID)
}

func (e *environSuite) setupVcnExpectations(vcnId string, t map[string]string, times int) {
	vcnName := makeVcnName(t[tags.JujuController], t[tags.JujuModel])
	vcnRequest, vcnResponse := makeListVcnRequestResponse([]ociCore.Vcn{
		{
			CompartmentId:         &e.testCompartment,
			CidrBlock:             makeStringPointer(oci.DefaultAddressSpace),
			Id:                    &vcnId,
			LifecycleState:        ociCore.VcnLifecycleStateAvailable,
			DefaultRouteTableId:   makeStringPointer("fakeRouteTable"),
			DefaultSecurityListId: makeStringPointer("fakeSeclist"),
			DisplayName:           &vcnName,
			FreeformTags:          e.tags,
		},
	})

	expect := e.netw.EXPECT().ListVcns(context.Background(), vcnRequest).Return(vcnResponse, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (e *environSuite) setupSecurityListExpectations(vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("juju-seclist-%s-%s", t[tags.JujuController], t[tags.JujuModel])
	request, response := makeListSecurityListsRequestResponse([]ociCore.SecurityList{
		{
			CompartmentId: &e.testCompartment,
			VcnId:         &vcnId,
			Id:            makeStringPointer("fakeSecList"),
			DisplayName:   &name,
			FreeformTags:  t,
			EgressSecurityRules: []ociCore.EgressSecurityRule{
				{
					Destination: makeStringPointer(oci.AllowAllPrefix),
					Protocol:    makeStringPointer(oci.AllProtocols),
				},
			},
			IngressSecurityRules: []ociCore.IngressSecurityRule{
				{
					Source:   makeStringPointer(oci.AllowAllPrefix),
					Protocol: makeStringPointer(oci.AllProtocols),
				},
			},
		},
	})
	expect := e.fw.EXPECT().ListSecurityLists(context.Background(), request).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (e *environSuite) setupInternetGatewaysExpectations(vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("%s-%s", oci.InternetGatewayPrefix, t[tags.JujuController])
	enabled := true
	request, response := makeListInternetGatewaysRequestResponse([]ociCore.InternetGateway{
		{
			CompartmentId: &e.testCompartment,
			Id:            makeStringPointer("fakeGwId"),
			VcnId:         &vcnId,
			DisplayName:   &name,
			IsEnabled:     &enabled,
		},
	})
	expect := e.netw.EXPECT().ListInternetGateways(context.Background(), request).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (e *environSuite) setupListRouteTableExpectations(vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("%s-%s", oci.RouteTablePrefix, t[tags.JujuController])
	request, response := makeListRouteTableRequestResponse([]ociCore.RouteTable{
		{
			CompartmentId:  &e.testCompartment,
			Id:             makeStringPointer("fakeRouteTableId"),
			VcnId:          &vcnId,
			DisplayName:    &name,
			FreeformTags:   t,
			LifecycleState: ociCore.RouteTableLifecycleStateAvailable,
		},
	})
	expect := e.netw.EXPECT().ListRouteTables(context.Background(), request).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (e *environSuite) setupListSubnetsExpectations(vcnId, route string, t map[string]string, times int) {
	zone1 := "fakeZone1"
	zone2 := "fakeZone2"
	zone3 := "fakeZone3"
	displayNameZone1 := fmt.Sprintf("juju-%s-%s-%s", zone1, t[tags.JujuController], t[tags.JujuModel])
	displayNameZone2 := fmt.Sprintf("juju-%s-%s-%s", zone2, t[tags.JujuController], t[tags.JujuModel])
	displayNameZone3 := fmt.Sprintf("juju-%s-%s-%s", zone3, t[tags.JujuController], t[tags.JujuModel])
	request, response := makeListSubnetsRequestResponse([]ociCore.Subnet{
		{
			AvailabilityDomain: &zone1,
			CidrBlock:          makeStringPointer(oci.DefaultAddressSpace),
			CompartmentId:      &e.testCompartment,
			Id:                 makeStringPointer("fakeSubnetId1"),
			VcnId:              &vcnId,
			DisplayName:        &displayNameZone1,
			RouteTableId:       &route,
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			FreeformTags:       t,
		},
		{
			AvailabilityDomain: &zone2,
			CidrBlock:          makeStringPointer(oci.DefaultAddressSpace),
			CompartmentId:      &e.testCompartment,
			Id:                 makeStringPointer("fakeSubnetId2"),
			VcnId:              &vcnId,
			DisplayName:        &displayNameZone2,
			RouteTableId:       &route,
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			FreeformTags:       t,
		},
		{
			AvailabilityDomain: &zone3,
			CidrBlock:          makeStringPointer(oci.DefaultAddressSpace),
			CompartmentId:      &e.testCompartment,
			Id:                 makeStringPointer("fakeSubnetId3"),
			VcnId:              &vcnId,
			DisplayName:        &displayNameZone3,
			RouteTableId:       &route,
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			FreeformTags:       t,
		},
	})

	expect := e.netw.EXPECT().ListSubnets(context.Background(), request).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (e *environSuite) setupListImagesExpectations() {
	request, response := makeListImageRequestResponse([]ociCore.Image{
		{
			CompartmentId:          &e.testCompartment,
			Id:                     makeStringPointer("fakeUbuntu1"),
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("14.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-2018.01.11-0"),
		},
		{
			CompartmentId:          &e.testCompartment,
			Id:                     makeStringPointer("fakeUbuntu2"),
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("14.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-2018.01.12-0"),
		},
		{
			CompartmentId:          &e.testCompartment,
			Id:                     makeStringPointer("fakeCentOS"),
			OperatingSystem:        makeStringPointer("CentOS"),
			OperatingSystemVersion: makeStringPointer("7"),
			DisplayName:            makeStringPointer("CentOS-7-2017.10.19-0"),
		},
	})
	_, shapesResponse := makeShapesRequestResponse(
		e.testCompartment, "fake", []string{
			"VM.Standard1.1",
		})
	e.compute.EXPECT().ListImages(context.Background(), request).Return(response, nil)
	e.compute.EXPECT().ListShapes(context.Background(), gomock.Any()).Return(shapesResponse, nil).AnyTimes()
}

func (e *environSuite) TestAvailabilityZones(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.setupAvailabilityDomainsExpectations(1)

	az, err := e.env.AvailabilityZones(nil)
	c.Assert(err, gc.IsNil)
	c.Check(len(az), gc.Equals, 3)
}

func (e *environSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.compute.EXPECT().ListInstances(
		context.Background(), e.listInstancesRequest).Return(
		e.listInstancesResponse, nil).Times(2)

	req := []instance.Id{
		instance.Id("fakeInstance1"),
	}
	zones, err := e.env.InstanceAvailabilityZoneNames(nil, req)
	c.Assert(err, gc.IsNil)
	c.Check(len(zones), gc.Equals, 1)
	c.Assert(zones[0], gc.Equals, "fakeZone1")

	req = []instance.Id{
		instance.Id("fakeInstance1"),
		instance.Id("fakeInstance3"),
	}
	zones, err = e.env.InstanceAvailabilityZoneNames(nil, req)
	c.Assert(err, gc.ErrorMatches, "only some instances were found")
	c.Check(len(zones), gc.Equals, 1)
}

func (e *environSuite) TestInstances(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.compute.EXPECT().ListInstances(
		context.Background(), e.listInstancesRequest).Return(
		e.listInstancesResponse, nil).Times(2)

	req := []instance.Id{
		instance.Id("fakeInstance1"),
	}

	inst, err := e.env.Instances(nil, req)
	c.Assert(err, gc.IsNil)
	c.Assert(len(inst), gc.Equals, 1)
	c.Assert(inst[0].Id(), gc.Equals, instance.Id("fakeInstance1"))

	req = []instance.Id{
		instance.Id("fakeInstance1"),
		instance.Id("fakeInstance3"),
	}
	inst, err = e.env.Instances(nil, req)
	c.Assert(err, gc.ErrorMatches, "only some instances were found")
	c.Check(len(inst), gc.Equals, 1)
	c.Assert(inst[0].Id(), gc.Equals, instance.Id("fakeInstance1"))
}

func (e *environSuite) TestPrepareForBootstrap(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.setupAvailabilityDomainsExpectations(1)
	e.ident.EXPECT().ListAvailabilityDomains(
		gomock.Any(), gomock.Any()).Return(
		ociIdentity.ListAvailabilityDomainsResponse{}, errors.New("got error"))

	ctx := envtesting.BootstrapContext(c)
	err := e.env.PrepareForBootstrap(ctx)
	c.Assert(err, gc.IsNil)

	err = e.env.PrepareForBootstrap(ctx)
	c.Assert(err, gc.ErrorMatches, "got error")
}

func (e *environSuite) TestCreate(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.setupAvailabilityDomainsExpectations(1)
	e.ident.EXPECT().ListAvailabilityDomains(
		gomock.Any(), gomock.Any()).Return(
		ociIdentity.ListAvailabilityDomainsResponse{}, errors.New("got error"))

	err := e.env.Create(nil, environs.CreateParams{})
	c.Assert(err, gc.IsNil)

	err = e.env.Create(nil, environs.CreateParams{})
	c.Assert(err, gc.ErrorMatches, "got error")
}

func (e *environSuite) TestConstraintsValidator(c *gc.C) {
	validator, err := e.env.ConstraintsValidator(envcontext.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)

}

func (e *environSuite) TestConstraintsValidatorEmpty(c *gc.C) {
	validator, err := e.env.ConstraintsValidator(envcontext.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (e *environSuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	validator, err := e.env.ConstraintsValidator(envcontext.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, jc.SameContents, []string{"tags", "virt-type"})
}

func (e *environSuite) TestConstraintsValidatorWrongArch(c *gc.C) {
	validator, err := e.env.ConstraintsValidator(envcontext.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Check(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are:.*")
}

func (e *environSuite) TestControllerInstancesNoControllerInstances(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.compute.EXPECT().ListInstances(
		context.Background(), e.listInstancesRequest).Return(
		e.listInstancesResponse, nil)

	ids, err := e.env.ControllerInstances(nil, e.controllerUUID)
	c.Assert(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 0)
}

func (e *environSuite) TestControllerInstancesOneController(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.listInstancesResponse.Items[0].FreeformTags = e.ctrlTags
	e.compute.EXPECT().ListInstances(
		context.Background(), e.listInstancesRequest).Return(
		e.listInstancesResponse, nil)

	ids, err := e.env.ControllerInstances(nil, e.controllerUUID)
	c.Assert(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 1)
}

type instanceTermination struct {
	instanceId string
	err        error
}

type ociInstanceTermination struct {
	instance ociCore.Instance
	err      error
}

func (e *environSuite) setupStopInstanceExpectations(instancesDetails []instanceTermination) {
	exp := e.compute.EXPECT()
	instancesListWithError := []ociInstanceTermination{}
	instancesList := []ociCore.Instance{}

	for _, inst := range instancesDetails {
		ociInstance := ociCore.Instance{
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &e.testCompartment,
			Id:                 makeStringPointer(inst.instanceId),
			LifecycleState:     ociCore.InstanceLifecycleStateRunning,
			Region:             makeStringPointer("us-phoenix-1"),
			Shape:              makeStringPointer("VM.Standard1.1"),
			DisplayName:        makeStringPointer("fakeName"),
			FreeformTags:       e.tags,
		}
		instancesListWithError = append(
			instancesListWithError,
			ociInstanceTermination{
				instance: ociInstance,
				err:      inst.err})
		instancesList = append(instancesList, ociInstance)
	}

	listInstancesRequest, listInstancesResponse := makeListInstancesRequestResponse(instancesList)

	listInstancesResponse.RawResponse = &http.Response{
		StatusCode: 200,
	}

	listCall := exp.ListInstances(
		context.Background(), listInstancesRequest).Return(
		listInstancesResponse, nil).AnyTimes()

	for _, inst := range instancesListWithError {
		requestMachine, responseMachine := makeGetInstanceRequestResponse(inst.instance)

		responseMachine.RawResponse = &http.Response{
			StatusCode: 200,
		}

		terminateRequestMachine := ociCore.TerminateInstanceRequest{
			InstanceId: inst.instance.Id,
		}

		terminateResponse := ociCore.TerminateInstanceResponse{
			RawResponse: &http.Response{
				StatusCode: 201,
			},
		}

		terminatingInst := inst.instance
		terminatingInst.LifecycleState = ociCore.InstanceLifecycleStateTerminating
		requestMachineTerminating, responseMachineTerminating := makeGetInstanceRequestResponse(terminatingInst)

		terminatedInst := inst.instance
		terminatedInst.LifecycleState = ociCore.InstanceLifecycleStateTerminated
		requestMachineTerminated, responseMachineTerminated := makeGetInstanceRequestResponse(terminatedInst)

		getCall := exp.GetInstance(
			context.Background(), requestMachine).Return(
			responseMachine, nil).AnyTimes().After(listCall)

		terminateCall := exp.TerminateInstance(
			context.Background(), terminateRequestMachine).Return(
			terminateResponse, inst.err).After(getCall)

		if inst.err == nil {
			terminatingCall := exp.GetInstance(
				context.Background(), requestMachineTerminating).Return(
				responseMachineTerminating, nil).Times(2).After(terminateCall)
			exp.GetInstance(
				context.Background(), requestMachineTerminated).Return(
				responseMachineTerminated, nil).After(terminatingCall)
		} else {
			exp.GetInstance(
				context.Background(), requestMachine).Return(
				responseMachine, nil).AnyTimes().After(terminateCall)
		}
	}
}

func (e *environSuite) TestStopInstances(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.setupStopInstanceExpectations(
		[]instanceTermination{
			{
				instanceId: "instance1",
				err:        nil,
			},
		},
	)

	ids := []instance.Id{
		instance.Id("instance1"),
	}
	err := e.env.StopInstances(nil, ids...)
	c.Assert(err, gc.IsNil)

}

func (e *environSuite) TestStopInstancesSingleFail(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.setupStopInstanceExpectations(
		[]instanceTermination{
			{
				instanceId: "fakeInstance1",
				err:        nil,
			},
			{
				instanceId: "fakeInstance2",
				err:        errors.Errorf("I failed to terminate"),
			},
			{
				instanceId: "fakeInstance3",
				err:        nil,
			},
		},
	)

	ids := []instance.Id{
		instance.Id("fakeInstance1"),
		instance.Id("fakeInstance2"),
		instance.Id("fakeInstance3"),
	}
	err := e.env.StopInstances(nil, ids...)
	c.Assert(err, gc.ErrorMatches, "failed to stop instance fakeInstance2: I failed to terminate")

}

func (e *environSuite) TestStopInstancesMultipleFail(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.setupStopInstanceExpectations(
		[]instanceTermination{
			{
				instanceId: "fakeInstance1",
				err:        nil,
			},
			{
				instanceId: "fakeInstance2",
				err:        errors.Errorf("I failed to terminate fakeInstance2"),
			},
			{
				instanceId: "fakeInstance3",
				err:        nil,
			},
			{
				instanceId: "fakeInstance4",
				err:        errors.Errorf("I failed to terminate fakeInstance4"),
			},
		},
	)

	ids := []instance.Id{
		instance.Id("fakeInstance1"),
		instance.Id("fakeInstance2"),
		instance.Id("fakeInstance3"),
		instance.Id("fakeInstance4"),
	}
	err := e.env.StopInstances(nil, ids...)
	// order in which the instances are returned or fail is not guaranteed
	c.Assert(err, gc.ErrorMatches, `failed to stop instances \[fakeInstance[24] fakeInstance[24]\]: \[I failed to terminate fakeInstance[24] I failed to terminate fakeInstance[24]\]`)

}

func (e *environSuite) TestStopInstancesTimeoutTransitioningToTerminating(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	listInstancesRequest, listInstancesResponse := makeListInstancesRequestResponse(
		[]ociCore.Instance{
			{
				AvailabilityDomain: makeStringPointer("fakeZone1"),
				CompartmentId:      &e.testCompartment,
				Id:                 makeStringPointer("fakeInstance1"),
				LifecycleState:     ociCore.InstanceLifecycleStateRunning,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName"),
				FreeformTags:       e.tags,
			},
		},
	)

	requestMachine1, responseMachine1 := makeGetInstanceRequestResponse(ociCore.Instance{
		CompartmentId:      listInstancesResponse.Items[0].CompartmentId,
		AvailabilityDomain: listInstancesResponse.Items[0].AvailabilityDomain,
		Id:                 listInstancesResponse.Items[0].Id,
		Region:             listInstancesResponse.Items[0].Region,
		Shape:              listInstancesResponse.Items[0].Shape,
		DisplayName:        listInstancesResponse.Items[0].DisplayName,
		FreeformTags:       listInstancesResponse.Items[0].FreeformTags,
		LifecycleState:     ociCore.InstanceLifecycleStateRunning,
	})

	e.listInstancesResponse.RawResponse = &http.Response{
		StatusCode: 200,
	}
	responseMachine1.RawResponse = &http.Response{
		StatusCode: 200,
	}

	terminateRequestMachine1 := ociCore.TerminateInstanceRequest{
		InstanceId: listInstancesResponse.Items[0].Id,
	}

	terminateResponse := ociCore.TerminateInstanceResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}

	gomock.InOrder(
		e.compute.EXPECT().ListInstances(
			context.Background(), listInstancesRequest).Return(
			listInstancesResponse, nil),
		e.compute.EXPECT().GetInstance(
			context.Background(), requestMachine1).Return(
			responseMachine1, nil),
		e.compute.EXPECT().TerminateInstance(
			context.Background(), terminateRequestMachine1).Return(
			terminateResponse, nil),
		e.compute.EXPECT().GetInstance(
			context.Background(), requestMachine1).Return(
			responseMachine1, nil).Times(3),
	)

	ids := []instance.Id{
		instance.Id("fakeInstance1"),
	}
	err := e.env.StopInstances(nil, ids...)
	c.Check(err, gc.ErrorMatches, ".*Instance still in running state after 2 checks")

}

func (e *environSuite) TestStopInstancesTimeoutTransitioningToTerminated(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	listInstancesRequest, listInstancesResponse := makeListInstancesRequestResponse(
		[]ociCore.Instance{
			{
				AvailabilityDomain: makeStringPointer("fakeZone1"),
				CompartmentId:      &e.testCompartment,
				Id:                 makeStringPointer("fakeInstance2"),
				LifecycleState:     ociCore.InstanceLifecycleStateRunning,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName"),
				FreeformTags:       e.tags,
			},
		},
	)

	requestMachine1, responseMachine1 := makeGetInstanceRequestResponse(ociCore.Instance{
		CompartmentId:      listInstancesResponse.Items[0].CompartmentId,
		AvailabilityDomain: listInstancesResponse.Items[0].AvailabilityDomain,
		Id:                 listInstancesResponse.Items[0].Id,
		Region:             listInstancesResponse.Items[0].Region,
		Shape:              listInstancesResponse.Items[0].Shape,
		DisplayName:        listInstancesResponse.Items[0].DisplayName,
		FreeformTags:       listInstancesResponse.Items[0].FreeformTags,
		LifecycleState:     ociCore.InstanceLifecycleStateRunning,
	})

	listInstancesResponse.RawResponse = &http.Response{
		StatusCode: 200,
	}
	responseMachine1.RawResponse = &http.Response{
		StatusCode: 200,
	}

	terminateRequestMachine1 := ociCore.TerminateInstanceRequest{
		InstanceId: listInstancesResponse.Items[0].Id,
	}

	terminateResponse := ociCore.TerminateInstanceResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}

	responseMachine1Terminating := responseMachine1
	responseMachine1Terminating.Instance.LifecycleState = ociCore.InstanceLifecycleStateTerminating

	gomock.InOrder(
		e.compute.EXPECT().ListInstances(
			context.Background(), listInstancesRequest).Return(
			listInstancesResponse, nil),
		e.compute.EXPECT().GetInstance(
			context.Background(), requestMachine1).Return(
			responseMachine1, nil),
		e.compute.EXPECT().TerminateInstance(
			context.Background(), terminateRequestMachine1).Return(
			terminateResponse, nil),
		e.compute.EXPECT().GetInstance(
			context.Background(), requestMachine1).Return(
			responseMachine1Terminating, nil).AnyTimes(),
	)

	ids := []instance.Id{
		instance.Id("fakeInstance2"),
	}
	err := e.env.StopInstances(nil, ids...)
	c.Check(err, gc.ErrorMatches, ".*Timed out waiting for instance to transition from TERMINATING to TERMINATED")

}

func (e *environSuite) TestAllInstances(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.compute.EXPECT().ListInstances(
		context.Background(), e.listInstancesRequest).Return(
		e.listInstancesResponse, nil)

	ids, err := e.env.AllInstances(nil)
	c.Assert(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 2)
}

func (e *environSuite) TestAllInstancesExtraUnrelatedInstance(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	// This instance does not have the model tags. It should be ignored.
	unrelatedInstance := newFakeOCIInstance(
		"notRelated", e.testCompartment, ociCore.InstanceLifecycleStateRunning)
	e.listInstancesResponse.Items = append(
		e.listInstancesResponse.Items, *unrelatedInstance)

	e.compute.EXPECT().ListInstances(
		context.Background(), e.listInstancesRequest).Return(
		e.listInstancesResponse, nil)

	ids, err := e.env.AllInstances(nil)
	c.Assert(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 2)
}

func (e *environSuite) setupLaunchInstanceExpectations(isController bool, tags map[string]string) {
	inst := ociCore.Instance{
		AvailabilityDomain: makeStringPointer("fakeZone1"),
		CompartmentId:      &e.testCompartment,
		Id:                 makeStringPointer("fakeInstanceId"),
		LifecycleState:     ociCore.InstanceLifecycleStateProvisioning,
		Region:             makeStringPointer("us-phoenix-1"),
		Shape:              makeStringPointer("VM.Standard1.1"),
		DisplayName:        makeStringPointer("juju-06f00d-0"),
		FreeformTags:       tags,
	}
	responseLaunch := ociCore.LaunchInstanceResponse{
		Instance: inst,
	}
	e.compute.EXPECT().LaunchInstance(context.Background(), gomock.Any()).Return(responseLaunch, nil)

	getInst := inst
	if isController {
		getInst.LifecycleState = ociCore.InstanceLifecycleStateRunning

	}
	getResponse := ociCore.GetInstanceResponse{
		Instance: getInst,
	}
	e.compute.EXPECT().GetInstance(context.Background(), gomock.Any()).Return(getResponse, nil)

	if isController {
		vnicID := "fakeVnicId"
		attachRequest, attachResponse := makeListVnicAttachmentsRequestResponse([]ociCore.VnicAttachment{
			{
				Id:                 makeStringPointer("fakeAttachmentId"),
				AvailabilityDomain: makeStringPointer("fake"),
				CompartmentId:      &e.testCompartment,
				InstanceId:         makeStringPointer("fakeInstanceId"),
				LifecycleState:     ociCore.VnicAttachmentLifecycleStateAttached,
				DisplayName:        makeStringPointer("fakeAttachmentName"),
				NicIndex:           makeIntPointer(0),
				VnicId:             &vnicID,
			},
		})

		vnicRequest, vnicResponse := makeGetVnicRequestResponse([]ociCore.GetVnicResponse{
			{
				Vnic: ociCore.Vnic{
					Id:             &vnicID,
					PrivateIp:      makeStringPointer("10.0.0.20"),
					DisplayName:    makeStringPointer("fakeVnicName"),
					PublicIp:       makeStringPointer("2.2.2.2"),
					MacAddress:     makeStringPointer("aa:aa:aa:aa:aa:aa"),
					SubnetId:       makeStringPointer("fakeSubnetId"),
					LifecycleState: ociCore.VnicLifecycleStateAvailable,
				},
			},
		})

		e.compute.EXPECT().ListVnicAttachments(context.Background(), attachRequest).Return(attachResponse, nil)
		e.netw.EXPECT().GetVnic(context.Background(), vnicRequest[0]).Return(vnicResponse[0], nil)
	}
}

func (e *environSuite) setupEnsureNetworksExpectations(vcnId string, machineTags map[string]string) {
	e.setupAvailabilityDomainsExpectations(0)
	e.setupVcnExpectations(vcnId, machineTags, 1)
	e.setupSecurityListExpectations(vcnId, machineTags, 1)
	e.setupInternetGatewaysExpectations(vcnId, machineTags, 1)
	e.setupListRouteTableExpectations(vcnId, machineTags, 1)
	e.setupListSubnetsExpectations(vcnId, "fakeRouteTableId", machineTags, 1)
}

func (e *environSuite) setupStartInstanceExpectations(isController bool) {
	vcnId := "fakeVCNId"
	machineTags := map[string]string{
		tags.JujuController: testing.ControllerTag.Id(),
		tags.JujuModel:      testing.ModelTag.Id(),
	}

	if isController {
		machineTags[tags.JujuIsController] = "true"
	}

	e.setupEnsureNetworksExpectations(vcnId, machineTags)
	e.setupListImagesExpectations()
	e.setupLaunchInstanceExpectations(isController, machineTags)
}

func (e *environSuite) TestBootstrap(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	e.setupStartInstanceExpectations(true)

	ctx := envtesting.BootstrapContext(c)
	_, err := e.env.Bootstrap(ctx, nil,
		environs.BootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			AvailableTools:   makeToolsList("trusty"),
			BootstrapSeries:  "trusty",
		})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestBootstrapNoMatchingTools(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	vcnId := "fakeVCNId"
	machineTags := map[string]string{
		tags.JujuController:   testing.ControllerTag.Id(),
		tags.JujuModel:        testing.ModelTag.Id(),
		tags.JujuIsController: "true",
	}

	e.setupAvailabilityDomainsExpectations(0)
	e.setupVcnExpectations(vcnId, machineTags, 0)
	e.setupSecurityListExpectations(vcnId, machineTags, 0)
	e.setupInternetGatewaysExpectations(vcnId, machineTags, 0)
	e.setupListRouteTableExpectations(vcnId, machineTags, 0)
	e.setupListSubnetsExpectations(vcnId, "fakeRouteTableId", machineTags, 0)

	ctx := envtesting.BootstrapContext(c)
	_, err := e.env.Bootstrap(ctx, nil,
		environs.BootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			AvailableTools:   makeToolsList("trusty"),
			BootstrapSeries:  "precise",
		})
	c.Assert(err, gc.ErrorMatches, "no matching agent binaries available")

}

func (e *environSuite) setupDeleteSecurityListExpectations(seclistId string, times int) {
	request := ociCore.DeleteSecurityListRequest{
		SecurityListId: makeStringPointer(seclistId),
	}

	response := ociCore.DeleteSecurityListResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}

	expect := e.fw.EXPECT().DeleteSecurityList(context.Background(), request).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}

	requestGet := ociCore.GetSecurityListRequest{
		SecurityListId: makeStringPointer("fakeSecList"),
	}

	seclist := ociCore.SecurityList{
		Id:             makeStringPointer("fakeSecList"),
		LifecycleState: ociCore.SecurityListLifecycleStateTerminated,
	}

	responseGet := ociCore.GetSecurityListResponse{
		SecurityList: seclist,
	}

	e.fw.EXPECT().GetSecurityList(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()

}

func (e *environSuite) setupDeleteSubnetExpectations(subnetIds []string) {
	for _, id := range subnetIds {
		request := ociCore.DeleteSubnetRequest{
			SubnetId: makeStringPointer(id),
		}

		response := ociCore.DeleteSubnetResponse{
			RawResponse: &http.Response{
				StatusCode: 201,
			},
		}
		e.netw.EXPECT().DeleteSubnet(context.Background(), request).Return(response, nil).AnyTimes()

		requestGet := ociCore.GetSubnetRequest{
			SubnetId: makeStringPointer(id),
		}

		subnet := ociCore.Subnet{
			Id:             makeStringPointer("fakeSecList"),
			LifecycleState: ociCore.SubnetLifecycleStateTerminated,
		}

		responseGet := ociCore.GetSubnetResponse{
			Subnet: subnet,
		}

		e.netw.EXPECT().GetSubnet(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()
	}
}

func (e *environSuite) setupDeleteRouteTableExpectations(vcnId, routeTableId string, t map[string]string) {
	e.setupListRouteTableExpectations(vcnId, t, 1)
	request := ociCore.DeleteRouteTableRequest{
		RtId: makeStringPointer(routeTableId),
	}

	response := ociCore.DeleteRouteTableResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	e.netw.EXPECT().DeleteRouteTable(context.Background(), request).Return(response, nil).AnyTimes()

	requestGet := ociCore.GetRouteTableRequest{
		RtId: makeStringPointer(routeTableId),
	}

	rt := ociCore.RouteTable{
		Id:             makeStringPointer(routeTableId),
		LifecycleState: ociCore.RouteTableLifecycleStateTerminated,
	}

	responseGet := ociCore.GetRouteTableResponse{
		RouteTable: rt,
	}

	e.netw.EXPECT().GetRouteTable(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (e *environSuite) setupDeleteInternetGatewayExpectations(vcnId, IgId string, t map[string]string) {
	e.setupInternetGatewaysExpectations(vcnId, t, 1)
	request := ociCore.DeleteInternetGatewayRequest{
		IgId: &IgId,
	}

	response := ociCore.DeleteInternetGatewayResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	e.netw.EXPECT().DeleteInternetGateway(context.Background(), request).Return(response, nil)

	requestGet := ociCore.GetInternetGatewayRequest{
		IgId: &IgId,
	}

	ig := ociCore.InternetGateway{
		Id:             &IgId,
		LifecycleState: ociCore.InternetGatewayLifecycleStateTerminated,
	}

	responseGet := ociCore.GetInternetGatewayResponse{
		InternetGateway: ig,
	}

	e.netw.EXPECT().GetInternetGateway(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (e *environSuite) setupDeleteVcnExpectations(vcnId string) {
	request := ociCore.DeleteVcnRequest{
		VcnId: &vcnId,
	}

	response := ociCore.DeleteVcnResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	e.netw.EXPECT().DeleteVcn(context.Background(), request).Return(response, nil)

	requestGet := ociCore.GetVcnRequest{
		VcnId: &vcnId,
	}

	vcn := ociCore.Vcn{
		Id:             &vcnId,
		LifecycleState: ociCore.VcnLifecycleStateTerminated,
	}

	responseGet := ociCore.GetVcnResponse{
		Vcn: vcn,
	}

	e.netw.EXPECT().GetVcn(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (e *environSuite) setupDeleteVolumesExpectations() {
	size := 50
	volumes := []ociCore.Volume{
		{
			Id:                 makeStringPointer("fakeVolumeID1"),
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &e.testCompartment,
			DisplayName:        makeStringPointer("fakeVolume1"),
			LifecycleState:     ociCore.VolumeLifecycleStateAvailable,
			SizeInGBs:          &size,
			FreeformTags: map[string]string{
				tags.JujuController: e.controllerUUID,
			},
		},
		{
			Id:                 makeStringPointer("fakeVolumeID2"),
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &e.testCompartment,
			DisplayName:        makeStringPointer("fakeVolume2"),
			LifecycleState:     ociCore.VolumeLifecycleStateAvailable,
			SizeInGBs:          &size,
			FreeformTags: map[string]string{
				tags.JujuController: e.controllerUUID,
			},
		},
	}

	copyVolumes := volumes
	copyVolumes[0].LifecycleState = ociCore.VolumeLifecycleStateTerminated
	copyVolumes[1].LifecycleState = ociCore.VolumeLifecycleStateTerminated

	listRequest := ociCore.ListVolumesRequest{
		CompartmentId: &e.testCompartment,
	}

	listResponse := ociCore.ListVolumesResponse{
		Items: volumes,
	}

	requestVolume1 := ociCore.GetVolumeRequest{
		VolumeId: makeStringPointer("fakeVolumeID1"),
	}

	requestVolume2 := ociCore.GetVolumeRequest{
		VolumeId: makeStringPointer("fakeVolumeID2"),
	}

	responseVolume1 := ociCore.GetVolumeResponse{
		Volume: copyVolumes[0],
	}

	responseVolume2 := ociCore.GetVolumeResponse{
		Volume: copyVolumes[1],
	}

	e.storage.EXPECT().ListVolumes(context.Background(), listRequest).Return(listResponse, nil).AnyTimes()
	e.storage.EXPECT().GetVolume(context.Background(), requestVolume1).Return(responseVolume1, nil).AnyTimes()
	e.storage.EXPECT().GetVolume(context.Background(), requestVolume2).Return(responseVolume2, nil).AnyTimes()
}

func (e *environSuite) TestDestroyController(c *gc.C) {
	ctrl := e.patchEnv(c)
	defer ctrl.Finish()

	machineTags := map[string]string{
		tags.JujuController: testing.ControllerTag.Id(),
		tags.JujuModel:      testing.ModelTag.Id(),
	}

	vcnId := "fakeVCNId"
	e.setupListInstancesExpectations(e.testInstanceID, ociCore.InstanceLifecycleStateRunning, 1)
	e.setupStopInstanceExpectations(
		[]instanceTermination{
			{
				instanceId: e.testInstanceID,
				err:        nil,
			},
		},
	)
	e.setupListInstancesExpectations(e.testInstanceID, ociCore.InstanceLifecycleStateTerminated, 0)
	e.setupVcnExpectations(vcnId, machineTags, 1)
	e.setupListSubnetsExpectations(vcnId, "fakeRouteTableId", machineTags, 1)
	e.setupSecurityListExpectations(vcnId, machineTags, 1)
	e.setupDeleteRouteTableExpectations(vcnId, "fakeRouteTableId", machineTags)
	e.setupDeleteSubnetExpectations([]string{"fakeSubnetId1", "fakeSubnetId2", "fakeSubnetId3"})
	e.setupDeleteSecurityListExpectations("fakeSecList", 0)
	e.setupDeleteInternetGatewayExpectations(vcnId, "fakeGwId", machineTags)
	e.setupDeleteVcnExpectations(vcnId)
	e.setupDeleteVolumesExpectations()

	err := e.env.DestroyController(nil, e.controllerUUID)
	c.Assert(err, gc.IsNil)
}
