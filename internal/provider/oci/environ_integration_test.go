// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	ociIdentity "github.com/oracle/oci-go-sdk/v65/identity"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/oci"
	"github.com/juju/juju/internal/testing"
)

type environSuite struct {
	commonSuite

	listInstancesResponse []ociCore.Instance
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)
	*oci.MaxPollIterations = 2
	s.listInstancesResponse =
		[]ociCore.Instance{
			{
				AvailabilityDomain: makeStringPointer("fakeZone1"),
				CompartmentId:      &s.testCompartment,
				Id:                 makeStringPointer("fakeInstance1"),
				LifecycleState:     ociCore.InstanceLifecycleStateRunning,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName"),
				FreeformTags:       s.tags,
			},
			{
				AvailabilityDomain: makeStringPointer("fakeZone2"),
				CompartmentId:      &s.testCompartment,
				Id:                 makeStringPointer("fakeInstance2"),
				LifecycleState:     ociCore.InstanceLifecycleStateRunning,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName2"),
				FreeformTags:       s.tags,
			},
		}

}

func (s *environSuite) setupAvailabilityDomainsExpectations(times int) {
	request, response := makeListAvailabilityDomainsRequestResponse([]ociIdentity.AvailabilityDomain{
		{
			Name:          makeStringPointer("fakeZone1"),
			CompartmentId: &s.testCompartment,
		},
		{
			Name:          makeStringPointer("fakeZone2"),
			CompartmentId: &s.testCompartment,
		},
		{
			Name:          makeStringPointer("fakeZone3"),
			CompartmentId: &s.testCompartment,
		},
	})

	expect := s.ident.EXPECT().ListAvailabilityDomains(context.Background(), request).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func makeVcnName(controllerUUID, modelUUID string) string {
	return fmt.Sprintf("%s-%s-%s", oci.VcnNamePrefix, controllerUUID, modelUUID)
}

func (s *environSuite) setupVcnExpectations(vcnId string, t map[string]string, times int) {
	vcnName := makeVcnName(t[tags.JujuController], t[tags.JujuModel])
	vcnResponse := []ociCore.Vcn{
		{
			CompartmentId:         &s.testCompartment,
			CidrBlock:             makeStringPointer(oci.DefaultAddressSpace),
			Id:                    &vcnId,
			LifecycleState:        ociCore.VcnLifecycleStateAvailable,
			DefaultRouteTableId:   makeStringPointer("fakeRouteTable"),
			DefaultSecurityListId: makeStringPointer("fakeSeclist"),
			DisplayName:           &vcnName,
			FreeformTags:          s.tags,
		},
	}

	expect := s.netw.EXPECT().ListVcns(context.Background(), &s.testCompartment).Return(vcnResponse, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupSecurityListExpectations(vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("juju-seclist-%s-%s", t[tags.JujuController], t[tags.JujuModel])
	request, response := makeListSecurityListsRequestResponse([]ociCore.SecurityList{
		{
			CompartmentId: &s.testCompartment,
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
	expect := s.fw.EXPECT().ListSecurityLists(context.Background(), request.CompartmentId, &vcnId).Return(response.Items, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupInternetGatewaysExpectations(vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("%s-%s", oci.InternetGatewayPrefix, t[tags.JujuController])
	enabled := true
	request, response := makeListInternetGatewaysRequestResponse([]ociCore.InternetGateway{
		{
			CompartmentId: &s.testCompartment,
			Id:            makeStringPointer("fakeGwId"),
			VcnId:         &vcnId,
			DisplayName:   &name,
			IsEnabled:     &enabled,
		},
	})
	expect := s.netw.EXPECT().ListInternetGateways(context.Background(), request.CompartmentId, request.VcnId).Return(response.Items, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupListRouteTableExpectations(vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("%s-%s", oci.RouteTablePrefix, t[tags.JujuController])
	request, response := makeListRouteTableRequestResponse([]ociCore.RouteTable{
		{
			CompartmentId:  &s.testCompartment,
			Id:             makeStringPointer("fakeRouteTableId"),
			VcnId:          &vcnId,
			DisplayName:    &name,
			FreeformTags:   t,
			LifecycleState: ociCore.RouteTableLifecycleStateAvailable,
		},
	})
	expect := s.netw.EXPECT().ListRouteTables(context.Background(), request.CompartmentId, request.VcnId).Return(response.Items, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupListSubnetsExpectations(vcnId, route string, t map[string]string, times int) {
	zone1 := "fakeZone1"
	zone2 := "fakeZone2"
	zone3 := "fakeZone3"
	displayNameZone1 := fmt.Sprintf("juju-%s-%s-%s", zone1, t[tags.JujuController], t[tags.JujuModel])
	displayNameZone2 := fmt.Sprintf("juju-%s-%s-%s", zone2, t[tags.JujuController], t[tags.JujuModel])
	displayNameZone3 := fmt.Sprintf("juju-%s-%s-%s", zone3, t[tags.JujuController], t[tags.JujuModel])
	response := []ociCore.Subnet{
		{
			AvailabilityDomain: &zone1,
			CidrBlock:          makeStringPointer(oci.DefaultAddressSpace),
			CompartmentId:      &s.testCompartment,
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
			CompartmentId:      &s.testCompartment,
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
			CompartmentId:      &s.testCompartment,
			Id:                 makeStringPointer("fakeSubnetId3"),
			VcnId:              &vcnId,
			DisplayName:        &displayNameZone3,
			RouteTableId:       &route,
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			FreeformTags:       t,
		},
	}

	expect := s.netw.EXPECT().ListSubnets(context.Background(), &s.testCompartment, &vcnId).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupListImagesExpectations() {
	response := []ociCore.Image{
		{
			CompartmentId:          &s.testCompartment,
			Id:                     makeStringPointer("fakeUbuntu1"),
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     makeStringPointer("fakeUbuntu2"),
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.12-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     makeStringPointer("fakeCentOS"),
			OperatingSystem:        makeStringPointer("CentOS"),
			OperatingSystemVersion: makeStringPointer("7"),
			DisplayName:            makeStringPointer("CentOS-7-2017.10.19-0"),
		},
	}
	s.compute.EXPECT().ListImages(context.Background(), &s.testCompartment).Return(response, nil)
	s.compute.EXPECT().ListShapes(context.Background(), gomock.Any(), gomock.Any()).Return(listShapesResponse(), nil).AnyTimes()
}

func (s *environSuite) TestAvailabilityZones(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupAvailabilityDomainsExpectations(1)

	az, err := s.env.AvailabilityZones(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, gc.IsNil)
	c.Check(len(az), gc.Equals, 3)
}

func (s *environSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().ListInstances(
		context.Background(), &s.testCompartment).Return(
		s.listInstancesResponse, nil).Times(2)

	id := instance.Id("fakeInstance1")
	req := []instance.Id{
		id,
	}
	zones, err := s.env.InstanceAvailabilityZoneNames(envcontext.WithoutCredentialInvalidator(context.Background()), req)
	c.Assert(err, gc.IsNil)
	c.Check(len(zones), gc.Equals, 1)
	c.Assert(zones[id], gc.Equals, "fakeZone1")

	req = []instance.Id{
		instance.Id("fakeInstance1"),
		instance.Id("fakeInstance3"),
	}
	zones, err = s.env.InstanceAvailabilityZoneNames(envcontext.WithoutCredentialInvalidator(context.Background()), req)
	c.Assert(err, gc.ErrorMatches, "only some instances were found")
	c.Check(len(zones), gc.Equals, 1)
}

func (s *environSuite) TestInstances(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().ListInstances(
		context.Background(), &s.testCompartment).Return(
		s.listInstancesResponse, nil).Times(2)

	req := []instance.Id{
		instance.Id("fakeInstance1"),
	}

	inst, err := s.env.Instances(envcontext.WithoutCredentialInvalidator(context.Background()), req)
	c.Assert(err, gc.IsNil)
	c.Assert(len(inst), gc.Equals, 1)
	c.Assert(inst[0].Id(), gc.Equals, instance.Id("fakeInstance1"))

	req = []instance.Id{
		instance.Id("fakeInstance1"),
		instance.Id("fakeInstance3"),
	}
	inst, err = s.env.Instances(envcontext.WithoutCredentialInvalidator(context.Background()), req)
	c.Assert(err, gc.ErrorMatches, "only some instances were found")
	c.Check(len(inst), gc.Equals, 1)
	c.Assert(inst[0].Id(), gc.Equals, instance.Id("fakeInstance1"))
}

func (s *environSuite) TestPrepareForBootstrap(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupAvailabilityDomainsExpectations(1)
	s.ident.EXPECT().ListAvailabilityDomains(
		gomock.Any(), gomock.Any()).Return(
		ociIdentity.ListAvailabilityDomainsResponse{}, errors.New("got error"))

	ctx := envtesting.BootstrapTestContext(c)
	err := s.env.PrepareForBootstrap(ctx, "controller-1")
	c.Assert(err, gc.IsNil)

	err = s.env.PrepareForBootstrap(ctx, "controller-1")
	c.Assert(err, gc.ErrorMatches, "got error")
}

func (s *environSuite) TestConstraintsValidator(c *gc.C) {
	validator, err := s.env.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)

}

func (s *environSuite) TestConstraintsValidatorEmpty(c *gc.C) {
	validator, err := s.env.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environSuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	validator, err := s.env.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, jc.SameContents, []string{"tags", "virt-type"})
}

func (s *environSuite) TestConstraintsValidatorWrongArch(c *gc.C) {
	validator, err := s.env.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Check(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are:.*")
}

func (s *environSuite) TestControllerInstancesNoControllerInstances(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().ListInstances(
		context.Background(), &s.testCompartment).Return(
		s.listInstancesResponse, nil)

	ids, err := s.env.ControllerInstances(envcontext.WithoutCredentialInvalidator(context.Background()), s.controllerUUID)
	c.Assert(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 0)
}

func (s *environSuite) TestControllerInstancesOneController(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.listInstancesResponse[0].FreeformTags = s.ctrlTags
	s.compute.EXPECT().ListInstances(
		context.Background(), &s.testCompartment).Return(
		s.listInstancesResponse, nil)

	ids, err := s.env.ControllerInstances(envcontext.WithoutCredentialInvalidator(context.Background()), s.controllerUUID)
	c.Assert(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 1)
}

func (s *environSuite) TestCloudInit(c *gc.C) {
	cfg, err := oci.GetCloudInitConfig(s.env, "ubuntu", 1234, 4321)
	c.Assert(err, jc.ErrorIsNil)
	script, err := cfg.RenderScript()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, jc.Contains, "/sbin/iptables -I INPUT -p tcp --dport 1234 -j ACCEPT")
	c.Check(script, jc.Contains, "/sbin/iptables -I INPUT -p tcp --dport 4321 -j ACCEPT")
	c.Check(script, jc.Contains, "/etc/init.d/netfilter-persistent save")

	cfg, err = oci.GetCloudInitConfig(s.env, "ubuntu", 0, 0)
	c.Assert(err, jc.ErrorIsNil)
	script, err = cfg.RenderScript()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(script, gc.Not(jc.Contains), "/sbin/iptables -I INPUT -p tcp --dport 1234 -j ACCEPT")
	c.Check(script, gc.Not(jc.Contains), "/sbin/iptables -I INPUT -p tcp --dport 4321 -j ACCEPT")
	c.Check(script, gc.Not(jc.Contains), "/etc/init.d/netfilter-persistent save")
}

type instanceTermination struct {
	instanceId string
	err        error
}

type ociInstanceTermination struct {
	instance ociCore.Instance
	err      error
}

func (s *environSuite) setupStopInstanceExpectations(instancesDetails []instanceTermination) {
	exp := s.compute.EXPECT()
	instancesListWithError := []ociInstanceTermination{}
	instancesList := []ociCore.Instance{}

	for _, inst := range instancesDetails {
		ociInstance := ociCore.Instance{
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &s.testCompartment,
			Id:                 makeStringPointer(inst.instanceId),
			LifecycleState:     ociCore.InstanceLifecycleStateRunning,
			Region:             makeStringPointer("us-phoenix-1"),
			Shape:              makeStringPointer("VM.Standard1.1"),
			DisplayName:        makeStringPointer("fakeName"),
			FreeformTags:       s.tags,
		}
		instancesListWithError = append(
			instancesListWithError,
			ociInstanceTermination{
				instance: ociInstance,
				err:      inst.err})
		instancesList = append(instancesList, ociInstance)
	}

	_, listInstancesResponse := makeListInstancesRequestResponse(instancesList)

	listInstancesResponse.RawResponse = &http.Response{
		StatusCode: 200,
	}

	listCall := exp.ListInstances(
		context.Background(), &s.testCompartment).Return(
		listInstancesResponse.Items, nil).AnyTimes()

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

func (s *environSuite) TestStopInstances(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStopInstanceExpectations(
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
	err := s.env.StopInstances(envcontext.WithoutCredentialInvalidator(context.Background()), ids...)
	c.Assert(err, gc.IsNil)

}

func (s *environSuite) TestStopInstancesSingleFail(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStopInstanceExpectations(
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
	err := s.env.StopInstances(envcontext.WithoutCredentialInvalidator(context.Background()), ids...)
	c.Assert(err, gc.ErrorMatches, "failed to stop instance fakeInstance2: I failed to terminate")

}

func (s *environSuite) TestStopInstancesMultipleFail(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStopInstanceExpectations(
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
	err := s.env.StopInstances(envcontext.WithoutCredentialInvalidator(context.Background()), ids...)
	// order in which the instances are returned or fail is not guaranteed
	c.Assert(err, gc.ErrorMatches, `failed to stop instances \[fakeInstance[24] fakeInstance[24]\]: \[I failed to terminate fakeInstance[24] I failed to terminate fakeInstance[24]\]`)

}

func (s *environSuite) TestStopInstancesTimeoutTransitioningToTerminating(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	listInstancesRequest, listInstancesResponse := makeListInstancesRequestResponse(
		[]ociCore.Instance{
			{
				AvailabilityDomain: makeStringPointer("fakeZone1"),
				CompartmentId:      &s.testCompartment,
				Id:                 makeStringPointer("fakeInstance1"),
				LifecycleState:     ociCore.InstanceLifecycleStateRunning,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName"),
				FreeformTags:       s.tags,
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

	//s.listInstancesResponse.RawResponse = &http.Response{
	//	StatusCode: 200,
	//}
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
		s.compute.EXPECT().ListInstances(
			context.Background(), listInstancesRequest.CompartmentId).Return(
			listInstancesResponse.Items, nil),
		s.compute.EXPECT().GetInstance(
			context.Background(), requestMachine1).Return(
			responseMachine1, nil),
		s.compute.EXPECT().TerminateInstance(
			context.Background(), terminateRequestMachine1).Return(
			terminateResponse, nil),
		s.compute.EXPECT().GetInstance(
			context.Background(), requestMachine1).Return(
			responseMachine1, nil).Times(3),
	)

	ids := []instance.Id{
		instance.Id("fakeInstance1"),
	}
	err := s.env.StopInstances(envcontext.WithoutCredentialInvalidator(context.Background()), ids...)
	c.Check(err, gc.ErrorMatches, ".*Instance still in running state after 2 checks")

}

func (s *environSuite) TestStopInstancesTimeoutTransitioningToTerminated(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	listInstancesRequest, listInstancesResponse := makeListInstancesRequestResponse(
		[]ociCore.Instance{
			{
				AvailabilityDomain: makeStringPointer("fakeZone1"),
				CompartmentId:      &s.testCompartment,
				Id:                 makeStringPointer("fakeInstance2"),
				LifecycleState:     ociCore.InstanceLifecycleStateRunning,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName"),
				FreeformTags:       s.tags,
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
		s.compute.EXPECT().ListInstances(
			context.Background(), listInstancesRequest.CompartmentId).Return(
			listInstancesResponse.Items, nil),
		s.compute.EXPECT().GetInstance(
			context.Background(), requestMachine1).Return(
			responseMachine1, nil),
		s.compute.EXPECT().TerminateInstance(
			context.Background(), terminateRequestMachine1).Return(
			terminateResponse, nil),
		s.compute.EXPECT().GetInstance(
			context.Background(), requestMachine1).Return(
			responseMachine1Terminating, nil).AnyTimes(),
	)

	ids := []instance.Id{
		instance.Id("fakeInstance2"),
	}
	err := s.env.StopInstances(envcontext.WithoutCredentialInvalidator(context.Background()), ids...)
	c.Check(err, gc.ErrorMatches, ".*Timed out waiting for instance to transition from TERMINATING to TERMINATED")

}

func (s *environSuite) TestAllRunningInstances(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().ListInstances(
		context.Background(), &s.testCompartment).Return(
		s.listInstancesResponse, nil)

	ids, err := s.env.AllRunningInstances(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 2)
}

func (s *environSuite) TestAllRunningInstancesExtraUnrelatedInstance(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	// This instance does not have the model tags. It should be ignored.
	unrelatedInstance := newFakeOCIInstance(
		"notRelated", s.testCompartment, ociCore.InstanceLifecycleStateRunning)
	s.listInstancesResponse = append(
		s.listInstancesResponse, *unrelatedInstance)

	s.compute.EXPECT().ListInstances(
		context.Background(), &s.testCompartment).Return(
		s.listInstancesResponse, nil)

	ids, err := s.env.AllRunningInstances(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 2)
}

func (s *environSuite) setupLaunchInstanceExpectations(
	isController bool, tags map[string]string, publicIP bool, launchInstanceMatcher gomock.Matcher,
) {
	inst := ociCore.Instance{
		AvailabilityDomain: makeStringPointer("fakeZone1"),
		CompartmentId:      &s.testCompartment,
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
	s.compute.EXPECT().LaunchInstance(context.Background(), launchInstanceMatcher).Return(responseLaunch, nil)

	getInst := inst
	if isController {
		getInst.LifecycleState = ociCore.InstanceLifecycleStateRunning

	}
	getResponse := ociCore.GetInstanceResponse{
		Instance: getInst,
	}
	s.compute.EXPECT().GetInstance(context.Background(), gomock.Any()).Return(getResponse, nil)

	if isController {
		vnicID := "fakeVnicId"
		attachRequest, attachResponse := makeListVnicAttachmentsRequestResponse([]ociCore.VnicAttachment{
			{
				Id:                 makeStringPointer("fakeAttachmentId"),
				AvailabilityDomain: makeStringPointer("fake"),
				CompartmentId:      &s.testCompartment,
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

		// These calls are only expected if we assign a public IP.
		// They occur when polling for the IP after the instance is started.
		if publicIP {
			s.compute.EXPECT().ListVnicAttachments(context.Background(), attachRequest.CompartmentId, makeStringPointer("fakeInstanceId")).Return(attachResponse.Items, nil)
			s.netw.EXPECT().GetVnic(context.Background(), vnicRequest[0]).Return(vnicResponse[0], nil)
		}
	}
}

func (s *environSuite) setupEnsureNetworksExpectations(vcnId string, machineTags map[string]string) {
	s.setupAvailabilityDomainsExpectations(0)
	s.setupVcnExpectations(vcnId, machineTags, 1)
	s.setupSecurityListExpectations(vcnId, machineTags, 1)
	s.setupInternetGatewaysExpectations(vcnId, machineTags, 1)
	s.setupListRouteTableExpectations(vcnId, machineTags, 1)
	s.setupListSubnetsExpectations(vcnId, "fakeRouteTableId", machineTags, 1)
}

func (s *environSuite) setupStartInstanceExpectations(
	isController bool, publicIP bool, launchInstanceMatcher gomock.Matcher) {
	vcnId := "fakeVCNId"
	machineTags := map[string]string{
		tags.JujuController: testing.ControllerTag.Id(),
		tags.JujuModel:      testing.ModelTag.Id(),
	}

	if isController {
		machineTags[tags.JujuIsController] = "true"
	}

	s.setupEnsureNetworksExpectations(vcnId, machineTags)
	s.setupListImagesExpectations()
	s.setupLaunchInstanceExpectations(isController, machineTags, publicIP, launchInstanceMatcher)
}

func (s *environSuite) TestBootstrap(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStartInstanceExpectations(true, true, gomock.Any())

	ctx := envtesting.BootstrapTestContext(c)
	_, err := s.env.Bootstrap(ctx, envcontext.WithoutCredentialInvalidator(context.Background()),
		environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           base.MustParseBaseFromString("ubuntu@22.04"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		})
	c.Assert(err, gc.IsNil)
}

func (s *environSuite) TestBootstrapFlexibleShape(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStartInstanceExpectations(true, true, gomock.Any())

	// By setting the constraint cpu-cores=32, we are selecting the
	// VM.Standard3.Flex shape defined in listShapesResponse(), which has
	// 32 maximum CPUs.
	ctx := envtesting.BootstrapTestContext(c)
	_, err := s.env.Bootstrap(ctx, envcontext.WithoutCredentialInvalidator(context.Background()),
		environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           base.MustParseBaseFromString("ubuntu@22.04"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
			BootstrapConstraints:    constraints.MustParse("cpu-cores=32"),
		})
	c.Assert(err, gc.IsNil)
}

type noPublicIPMatcher struct{}

func (noPublicIPMatcher) Matches(arg interface{}) bool {
	li := arg.(ociCore.LaunchInstanceRequest)
	assign := *li.CreateVnicDetails.AssignPublicIp
	return !assign
}

func (noPublicIPMatcher) String() string { return "" }

func (s *environSuite) TestBootstrapNoAllocatePublicIP(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStartInstanceExpectations(true, false, noPublicIPMatcher{})

	ctx := envtesting.BootstrapTestContext(c)
	_, err := s.env.Bootstrap(ctx, envcontext.WithoutCredentialInvalidator(context.Background()),
		environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("ubuntu"),
			BootstrapBase:           base.MustParseBaseFromString("ubuntu@22.04"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
			BootstrapConstraints:    constraints.MustParse("allocate-public-ip=false"),
		})
	c.Assert(err, gc.IsNil)
}

func (s *environSuite) TestBootstrapNoMatchingTools(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vcnId := "fakeVCNId"
	machineTags := map[string]string{
		tags.JujuController:   testing.ControllerTag.Id(),
		tags.JujuModel:        testing.ModelTag.Id(),
		tags.JujuIsController: "true",
	}

	s.setupAvailabilityDomainsExpectations(0)
	s.setupVcnExpectations(vcnId, machineTags, 0)
	s.setupSecurityListExpectations(vcnId, machineTags, 0)
	s.setupInternetGatewaysExpectations(vcnId, machineTags, 0)
	s.setupListRouteTableExpectations(vcnId, machineTags, 0)
	s.setupListSubnetsExpectations(vcnId, "fakeRouteTableId", machineTags, 0)

	ctx := envtesting.BootstrapTestContext(c)
	_, err := s.env.Bootstrap(ctx, envcontext.WithoutCredentialInvalidator(context.Background()),
		environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			AvailableTools:          makeToolsList("centos"),
			BootstrapBase:           base.MustParseBaseFromString("ubuntu@22.04"),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		})
	c.Assert(err, gc.ErrorMatches, "no matching agent binaries available")

}

func (s *environSuite) setupDeleteSecurityListExpectations(seclistId string, times int) {
	request := ociCore.DeleteSecurityListRequest{
		SecurityListId: makeStringPointer(seclistId),
	}

	response := ociCore.DeleteSecurityListResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}

	expect := s.fw.EXPECT().DeleteSecurityList(context.Background(), request).Return(response, nil)
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

	s.fw.EXPECT().GetSecurityList(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()

}

func (s *environSuite) setupDeleteSubnetExpectations(subnetIds []string) {
	for _, id := range subnetIds {
		request := ociCore.DeleteSubnetRequest{
			SubnetId: makeStringPointer(id),
		}

		response := ociCore.DeleteSubnetResponse{
			RawResponse: &http.Response{
				StatusCode: 201,
			},
		}
		s.netw.EXPECT().DeleteSubnet(context.Background(), request).Return(response, nil).AnyTimes()

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

		s.netw.EXPECT().GetSubnet(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()
	}
}

func (s *environSuite) setupDeleteRouteTableExpectations(vcnId, routeTableId string, t map[string]string) {
	s.setupListRouteTableExpectations(vcnId, t, 1)
	request := ociCore.DeleteRouteTableRequest{
		RtId: makeStringPointer(routeTableId),
	}

	response := ociCore.DeleteRouteTableResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	s.netw.EXPECT().DeleteRouteTable(context.Background(), request).Return(response, nil).AnyTimes()

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

	s.netw.EXPECT().GetRouteTable(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (s *environSuite) setupDeleteInternetGatewayExpectations(vcnId, igId string, t map[string]string) {
	s.setupInternetGatewaysExpectations(vcnId, t, 1)
	request := ociCore.DeleteInternetGatewayRequest{
		IgId: &igId,
	}

	response := ociCore.DeleteInternetGatewayResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	s.netw.EXPECT().DeleteInternetGateway(context.Background(), request).Return(response, nil)

	requestGet := ociCore.GetInternetGatewayRequest{
		IgId: &igId,
	}

	ig := ociCore.InternetGateway{
		Id:             &igId,
		LifecycleState: ociCore.InternetGatewayLifecycleStateTerminated,
	}

	responseGet := ociCore.GetInternetGatewayResponse{
		InternetGateway: ig,
	}

	s.netw.EXPECT().GetInternetGateway(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (s *environSuite) setupDeleteVcnExpectations(vcnId string) {
	request := ociCore.DeleteVcnRequest{
		VcnId: &vcnId,
	}

	response := ociCore.DeleteVcnResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	s.netw.EXPECT().DeleteVcn(context.Background(), request).Return(response, nil)

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

	s.netw.EXPECT().GetVcn(context.Background(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (s *environSuite) setupDeleteVolumesExpectations() {
	size := int64(50)
	volumes := []ociCore.Volume{
		{
			Id:                 makeStringPointer("fakeVolumeID1"),
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &s.testCompartment,
			DisplayName:        makeStringPointer("fakeVolume1"),
			LifecycleState:     ociCore.VolumeLifecycleStateAvailable,
			SizeInGBs:          &size,
			FreeformTags: map[string]string{
				tags.JujuController: s.controllerUUID,
			},
		},
		{
			Id:                 makeStringPointer("fakeVolumeID2"),
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &s.testCompartment,
			DisplayName:        makeStringPointer("fakeVolume2"),
			LifecycleState:     ociCore.VolumeLifecycleStateAvailable,
			SizeInGBs:          &size,
			FreeformTags: map[string]string{
				tags.JujuController: s.controllerUUID,
			},
		},
	}

	copyVolumes := volumes
	copyVolumes[0].LifecycleState = ociCore.VolumeLifecycleStateTerminated
	copyVolumes[1].LifecycleState = ociCore.VolumeLifecycleStateTerminated

	listRequest := ociCore.ListVolumesRequest{
		CompartmentId: &s.testCompartment,
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

	s.storage.EXPECT().ListVolumes(context.Background(), listRequest.CompartmentId).Return(listResponse.Items, nil).AnyTimes()
	s.storage.EXPECT().GetVolume(context.Background(), requestVolume1).Return(responseVolume1, nil).AnyTimes()
	s.storage.EXPECT().GetVolume(context.Background(), requestVolume2).Return(responseVolume2, nil).AnyTimes()
}

func (s *environSuite) TestDestroyController(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	machineTags := map[string]string{
		tags.JujuController: testing.ControllerTag.Id(),
		tags.JujuModel:      testing.ModelTag.Id(),
	}

	vcnId := "fakeVCNId"
	s.setupListInstancesExpectations(s.testInstanceID, ociCore.InstanceLifecycleStateRunning, 1)
	s.setupStopInstanceExpectations(
		[]instanceTermination{
			{
				instanceId: s.testInstanceID,
				err:        nil,
			},
		},
	)
	s.setupListInstancesExpectations(s.testInstanceID, ociCore.InstanceLifecycleStateTerminated, 0)
	s.setupVcnExpectations(vcnId, machineTags, 1)
	s.setupListSubnetsExpectations(vcnId, "fakeRouteTableId", machineTags, 1)
	s.setupSecurityListExpectations(vcnId, machineTags, 1)
	s.setupDeleteRouteTableExpectations(vcnId, "fakeRouteTableId", machineTags)
	s.setupDeleteSubnetExpectations([]string{"fakeSubnetId1", "fakeSubnetId2", "fakeSubnetId3"})
	s.setupDeleteSecurityListExpectations("fakeSecList", 0)
	s.setupDeleteInternetGatewayExpectations(vcnId, "fakeGwId", machineTags)
	s.setupDeleteVcnExpectations(vcnId)
	s.setupDeleteVolumesExpectations()

	err := s.env.DestroyController(envcontext.WithoutCredentialInvalidator(context.Background()), s.controllerUUID)
	c.Assert(err, gc.IsNil)
}

func (s *environSuite) TestEnsureShapeConfig(c *gc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	machineTags := map[string]string{
		tags.JujuController: testing.ControllerTag.Id(),
		tags.JujuModel:      testing.ModelTag.Id(),
	}

	vcnId := "fakeVCNId"
	s.setupListInstancesExpectations(s.testInstanceID, ociCore.InstanceLifecycleStateRunning, 1)
	s.setupStopInstanceExpectations(
		[]instanceTermination{
			{
				instanceId: s.testInstanceID,
				err:        nil,
			},
		},
	)
	s.setupListInstancesExpectations(s.testInstanceID, ociCore.InstanceLifecycleStateTerminated, 0)
	s.setupVcnExpectations(vcnId, machineTags, 1)
	s.setupListSubnetsExpectations(vcnId, "fakeRouteTableId", machineTags, 1)
	s.setupSecurityListExpectations(vcnId, machineTags, 1)
	s.setupDeleteRouteTableExpectations(vcnId, "fakeRouteTableId", machineTags)
	s.setupDeleteSubnetExpectations([]string{"fakeSubnetId1", "fakeSubnetId2", "fakeSubnetId3"})
	s.setupDeleteSecurityListExpectations("fakeSecList", 0)
	s.setupDeleteInternetGatewayExpectations(vcnId, "fakeGwId", machineTags)
	s.setupDeleteVcnExpectations(vcnId)
	s.setupDeleteVolumesExpectations()

	err := s.env.DestroyController(envcontext.WithoutCredentialInvalidator(context.Background()), s.controllerUUID)
	c.Assert(err, gc.IsNil)
}
