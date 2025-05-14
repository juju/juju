// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/tc"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	ociIdentity "github.com/oracle/oci-go-sdk/v65/identity"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/oci"
	"github.com/juju/juju/internal/testing"
)

type environSuite struct {
	commonSuite

	listInstancesResponse []ociCore.Instance
}

var _ = tc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *tc.C) {
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

func (s *environSuite) setupAvailabilityDomainsExpectations(c *tc.C, times int) {
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

	expect := s.ident.EXPECT().ListAvailabilityDomains(gomock.Any(), request).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func makeVcnName(controllerUUID, modelUUID string) string {
	return fmt.Sprintf("%s-%s-%s", oci.VcnNamePrefix, controllerUUID, modelUUID)
}

func (s *environSuite) setupVcnExpectations(c *tc.C, vcnId string, t map[string]string, times int) {
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

	expect := s.netw.EXPECT().ListVcns(gomock.Any(), &s.testCompartment).Return(vcnResponse, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupSecurityListExpectations(c *tc.C, vcnId string, t map[string]string, times int) {
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
	expect := s.fw.EXPECT().ListSecurityLists(gomock.Any(), request.CompartmentId, &vcnId).Return(response.Items, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupInternetGatewaysExpectations(c *tc.C, vcnId string, t map[string]string, times int) {
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
	expect := s.netw.EXPECT().ListInternetGateways(gomock.Any(), request.CompartmentId, request.VcnId).Return(response.Items, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupListRouteTableExpectations(c *tc.C, vcnId string, t map[string]string, times int) {
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
	expect := s.netw.EXPECT().ListRouteTables(gomock.Any(), request.CompartmentId, request.VcnId).Return(response.Items, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupListSubnetsExpectations(c *tc.C, vcnId, route string, t map[string]string, times int) {
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

	expect := s.netw.EXPECT().ListSubnets(gomock.Any(), &s.testCompartment, &vcnId).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *environSuite) setupListImagesExpectations(c *tc.C) {
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
	s.compute.EXPECT().ListImages(gomock.Any(), &s.testCompartment).Return(response, nil)
	s.compute.EXPECT().ListShapes(gomock.Any(), gomock.Any(), gomock.Any()).Return(listShapesResponse(), nil).AnyTimes()
}

func (s *environSuite) TestAvailabilityZones(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupAvailabilityDomainsExpectations(c, 1)

	az, err := s.env.AvailabilityZones(c.Context())
	c.Assert(err, tc.IsNil)
	c.Check(len(az), tc.Equals, 3)
}

func (s *environSuite) TestInstanceAvailabilityZoneNames(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().ListInstances(gomock.Any(), &s.testCompartment).Return(
		s.listInstancesResponse, nil).Times(2)

	id := instance.Id("fakeInstance1")
	req := []instance.Id{
		id,
	}
	zones, err := s.env.InstanceAvailabilityZoneNames(c.Context(), req)
	c.Assert(err, tc.IsNil)
	c.Check(len(zones), tc.Equals, 1)
	c.Assert(zones[id], tc.Equals, "fakeZone1")

	req = []instance.Id{
		instance.Id("fakeInstance1"),
		instance.Id("fakeInstance3"),
	}
	zones, err = s.env.InstanceAvailabilityZoneNames(c.Context(), req)
	c.Assert(err, tc.ErrorMatches, "only some instances were found")
	c.Check(len(zones), tc.Equals, 1)
}

func (s *environSuite) TestInstances(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().ListInstances(gomock.Any(), &s.testCompartment).Return(
		s.listInstancesResponse, nil).Times(2)

	req := []instance.Id{
		instance.Id("fakeInstance1"),
	}

	inst, err := s.env.Instances(c.Context(), req)
	c.Assert(err, tc.IsNil)
	c.Assert(len(inst), tc.Equals, 1)
	c.Assert(inst[0].Id(), tc.Equals, instance.Id("fakeInstance1"))

	req = []instance.Id{
		instance.Id("fakeInstance1"),
		instance.Id("fakeInstance3"),
	}
	inst, err = s.env.Instances(c.Context(), req)
	c.Assert(err, tc.ErrorMatches, "only some instances were found")
	c.Check(len(inst), tc.Equals, 1)
	c.Assert(inst[0].Id(), tc.Equals, instance.Id("fakeInstance1"))
}

func (s *environSuite) TestPrepareForBootstrap(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupAvailabilityDomainsExpectations(c, 1)
	s.ident.EXPECT().ListAvailabilityDomains(
		gomock.Any(), gomock.Any()).Return(
		ociIdentity.ListAvailabilityDomainsResponse{}, errors.New("got error"))

	ctx := envtesting.BootstrapTestContext(c)
	err := s.env.PrepareForBootstrap(ctx, "controller-1")
	c.Assert(err, tc.IsNil)

	err = s.env.PrepareForBootstrap(ctx, "controller-1")
	c.Assert(err, tc.ErrorMatches, "got error")
}

func (s *environSuite) TestConstraintsValidator(c *tc.C) {
	validator, err := s.env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unsupported, tc.HasLen, 0)

}

func (s *environSuite) TestConstraintsValidatorEmpty(c *tc.C) {
	validator, err := s.env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unsupported, tc.HasLen, 0)
}

func (s *environSuite) TestConstraintsValidatorUnsupported(c *tc.C) {
	validator, err := s.env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unsupported, tc.SameContents, []string{"tags", "virt-type"})
}

func (s *environSuite) TestConstraintsValidatorWrongArch(c *tc.C) {
	validator, err := s.env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Check(err, tc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are:.*")
}

func (s *environSuite) TestControllerInstancesNoControllerInstances(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().ListInstances(gomock.Any(), &s.testCompartment).Return(
		s.listInstancesResponse, nil)

	ids, err := s.env.ControllerInstances(c.Context(), s.controllerUUID)
	c.Assert(err, tc.IsNil)
	c.Check(len(ids), tc.Equals, 0)
}

func (s *environSuite) TestControllerInstancesOneController(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.listInstancesResponse[0].FreeformTags = s.ctrlTags
	s.compute.EXPECT().ListInstances(gomock.Any(), &s.testCompartment).Return(
		s.listInstancesResponse, nil)

	ids, err := s.env.ControllerInstances(c.Context(), s.controllerUUID)
	c.Assert(err, tc.IsNil)
	c.Check(len(ids), tc.Equals, 1)
}

func (s *environSuite) TestCloudInit(c *tc.C) {
	cfg, err := oci.GetCloudInitConfig(s.env, "ubuntu", 1234, 4321)
	c.Assert(err, tc.ErrorIsNil)
	script, err := cfg.RenderScript()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(script, tc.Contains, "/sbin/iptables -I INPUT -p tcp --dport 1234 -j ACCEPT")
	c.Check(script, tc.Contains, "/sbin/iptables -I INPUT -p tcp --dport 4321 -j ACCEPT")
	c.Check(script, tc.Contains, "/etc/init.d/netfilter-persistent save")

	cfg, err = oci.GetCloudInitConfig(s.env, "ubuntu", 0, 0)
	c.Assert(err, tc.ErrorIsNil)
	script, err = cfg.RenderScript()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(script, tc.Not(tc.Contains), "/sbin/iptables -I INPUT -p tcp --dport 1234 -j ACCEPT")
	c.Check(script, tc.Not(tc.Contains), "/sbin/iptables -I INPUT -p tcp --dport 4321 -j ACCEPT")
	c.Check(script, tc.Not(tc.Contains), "/etc/init.d/netfilter-persistent save")
}

type instanceTermination struct {
	instanceId string
	err        error
}

type ociInstanceTermination struct {
	instance ociCore.Instance
	err      error
}

func (s *environSuite) setupStopInstanceExpectations(c *tc.C, instancesDetails []instanceTermination) {
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

	listCall := s.compute.EXPECT().ListInstances(gomock.Any(), &s.testCompartment).Return(
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

		getCall := s.compute.EXPECT().GetInstance(gomock.Any(), requestMachine).Return(
			responseMachine, nil).AnyTimes().After(listCall)

		terminateCall := s.compute.EXPECT().TerminateInstance(gomock.Any(), terminateRequestMachine).Return(
			terminateResponse, inst.err).After(getCall)

		if inst.err == nil {
			terminatingCall := s.compute.EXPECT().GetInstance(gomock.Any(), requestMachineTerminating).Return(
				responseMachineTerminating, nil).Times(2).After(terminateCall)
			s.compute.EXPECT().GetInstance(gomock.Any(), requestMachineTerminated).Return(
				responseMachineTerminated, nil).After(terminatingCall)
		} else {
			s.compute.EXPECT().GetInstance(gomock.Any(), requestMachine).Return(
				responseMachine, nil).AnyTimes().After(terminateCall)
		}
	}
}

func (s *environSuite) TestStopInstances(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStopInstanceExpectations(c,
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
	err := s.env.StopInstances(c.Context(), ids...)
	c.Assert(err, tc.IsNil)

}

func (s *environSuite) TestStopInstancesSingleFail(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStopInstanceExpectations(c,
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
	err := s.env.StopInstances(c.Context(), ids...)
	c.Assert(err, tc.ErrorMatches, "failed to stop instance fakeInstance2: I failed to terminate")

}

func (s *environSuite) TestStopInstancesMultipleFail(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStopInstanceExpectations(c,
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
	err := s.env.StopInstances(c.Context(), ids...)
	// order in which the instances are returned or fail is not guaranteed
	c.Assert(err, tc.ErrorMatches, `failed to stop instances \[fakeInstance[24] fakeInstance[24]\]: \[I failed to terminate fakeInstance[24] I failed to terminate fakeInstance[24]\]`)

}

func (s *environSuite) TestStopInstancesTimeoutTransitioningToTerminating(c *tc.C) {
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
			gomock.Any(), listInstancesRequest.CompartmentId).Return(
			listInstancesResponse.Items, nil),
		s.compute.EXPECT().GetInstance(
			gomock.Any(), requestMachine1).Return(
			responseMachine1, nil),
		s.compute.EXPECT().TerminateInstance(
			gomock.Any(), terminateRequestMachine1).Return(
			terminateResponse, nil),
		s.compute.EXPECT().GetInstance(
			gomock.Any(), requestMachine1).Return(
			responseMachine1, nil).Times(3),
	)

	ids := []instance.Id{
		instance.Id("fakeInstance1"),
	}
	err := s.env.StopInstances(c.Context(), ids...)
	c.Check(err, tc.ErrorMatches, ".*Instance still in running state after 2 checks")

}

func (s *environSuite) TestStopInstancesTimeoutTransitioningToTerminated(c *tc.C) {
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
			gomock.Any(), listInstancesRequest.CompartmentId).Return(
			listInstancesResponse.Items, nil),
		s.compute.EXPECT().GetInstance(
			gomock.Any(), requestMachine1).Return(
			responseMachine1, nil),
		s.compute.EXPECT().TerminateInstance(
			gomock.Any(), terminateRequestMachine1).Return(
			terminateResponse, nil),
		s.compute.EXPECT().GetInstance(
			gomock.Any(), requestMachine1).Return(
			responseMachine1Terminating, nil).AnyTimes(),
	)

	ids := []instance.Id{
		instance.Id("fakeInstance2"),
	}
	err := s.env.StopInstances(c.Context(), ids...)
	c.Check(err, tc.ErrorMatches, ".*Timed out waiting for instance to transition from TERMINATING to TERMINATED")

}

func (s *environSuite) TestAllRunningInstances(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.compute.EXPECT().ListInstances(gomock.Any(), &s.testCompartment).Return(
		s.listInstancesResponse, nil)

	ids, err := s.env.AllRunningInstances(c.Context())
	c.Assert(err, tc.IsNil)
	c.Check(len(ids), tc.Equals, 2)
}

func (s *environSuite) TestAllRunningInstancesExtraUnrelatedInstance(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	// This instance does not have the model tags. It should be ignored.
	unrelatedInstance := newFakeOCIInstance(
		"notRelated", s.testCompartment, ociCore.InstanceLifecycleStateRunning)
	s.listInstancesResponse = append(
		s.listInstancesResponse, *unrelatedInstance)

	s.compute.EXPECT().ListInstances(
		gomock.Any(), &s.testCompartment).Return(
		s.listInstancesResponse, nil)

	ids, err := s.env.AllRunningInstances(c.Context())
	c.Assert(err, tc.IsNil)
	c.Check(len(ids), tc.Equals, 2)
}

func (s *environSuite) setupLaunchInstanceExpectations(
	c *tc.C, isController bool, tags map[string]string, publicIP bool, launchInstanceMatcher gomock.Matcher,
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
	s.compute.EXPECT().LaunchInstance(gomock.Any(), launchInstanceMatcher).Return(responseLaunch, nil)

	getInst := inst
	if isController {
		getInst.LifecycleState = ociCore.InstanceLifecycleStateRunning

	}
	getResponse := ociCore.GetInstanceResponse{
		Instance: getInst,
	}
	s.compute.EXPECT().GetInstance(gomock.Any(), gomock.Any()).Return(getResponse, nil)

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
			s.compute.EXPECT().ListVnicAttachments(gomock.Any(), attachRequest.CompartmentId, makeStringPointer("fakeInstanceId")).Return(attachResponse.Items, nil)
			s.netw.EXPECT().GetVnic(gomock.Any(), vnicRequest[0]).Return(vnicResponse[0], nil)
		}
	}
}

func (s *environSuite) setupEnsureNetworksExpectations(c *tc.C, vcnId string, machineTags map[string]string) {
	s.setupAvailabilityDomainsExpectations(c, 0)
	s.setupVcnExpectations(c, vcnId, machineTags, 1)
	s.setupSecurityListExpectations(c, vcnId, machineTags, 1)
	s.setupInternetGatewaysExpectations(c, vcnId, machineTags, 1)
	s.setupListRouteTableExpectations(c, vcnId, machineTags, 1)
	s.setupListSubnetsExpectations(c, vcnId, "fakeRouteTableId", machineTags, 1)
}

func (s *environSuite) setupStartInstanceExpectations(
	c *tc.C, isController bool, publicIP bool, launchInstanceMatcher gomock.Matcher) {
	vcnId := "fakeVCNId"
	machineTags := map[string]string{
		tags.JujuController: testing.ControllerTag.Id(),
		tags.JujuModel:      testing.ModelTag.Id(),
	}

	if isController {
		machineTags[tags.JujuIsController] = "true"
	}

	s.setupEnsureNetworksExpectations(c, vcnId, machineTags)
	s.setupListImagesExpectations(c)
	s.setupLaunchInstanceExpectations(c, isController, machineTags, publicIP, launchInstanceMatcher)
}

func (s *environSuite) TestBootstrap(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStartInstanceExpectations(c, true, true, gomock.Any())

	ctx := envtesting.BootstrapTestContext(c)
	_, err := s.env.Bootstrap(ctx, environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		AvailableTools:          makeToolsList("ubuntu"),
		BootstrapBase:           base.MustParseBaseFromString("ubuntu@22.04"),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.IsNil)
}

func (s *environSuite) TestBootstrapFlexibleShape(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStartInstanceExpectations(c, true, true, gomock.Any())

	// By setting the constraint cpu-cores=32, we are selecting the
	// VM.Standard3.Flex shape defined in listShapesResponse(), which has
	// 32 maximum CPUs.
	ctx := envtesting.BootstrapTestContext(c)
	_, err := s.env.Bootstrap(ctx, environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		AvailableTools:          makeToolsList("ubuntu"),
		BootstrapBase:           base.MustParseBaseFromString("ubuntu@22.04"),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		BootstrapConstraints:    constraints.MustParse("cpu-cores=32"),
	})
	c.Assert(err, tc.IsNil)
}

type noPublicIPMatcher struct{}

func (noPublicIPMatcher) Matches(arg interface{}) bool {
	li := arg.(ociCore.LaunchInstanceRequest)
	assign := *li.CreateVnicDetails.AssignPublicIp
	return !assign
}

func (noPublicIPMatcher) String() string { return "" }

func (s *environSuite) TestBootstrapNoAllocatePublicIP(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupStartInstanceExpectations(c, true, false, noPublicIPMatcher{})

	ctx := envtesting.BootstrapTestContext(c)
	_, err := s.env.Bootstrap(ctx, environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		AvailableTools:          makeToolsList("ubuntu"),
		BootstrapBase:           base.MustParseBaseFromString("ubuntu@22.04"),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		BootstrapConstraints:    constraints.MustParse("allocate-public-ip=false"),
	})
	c.Assert(err, tc.IsNil)
}

func (s *environSuite) TestBootstrapNoMatchingTools(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	vcnId := "fakeVCNId"
	machineTags := map[string]string{
		tags.JujuController:   testing.ControllerTag.Id(),
		tags.JujuModel:        testing.ModelTag.Id(),
		tags.JujuIsController: "true",
	}

	s.setupAvailabilityDomainsExpectations(c, 0)
	s.setupVcnExpectations(c, vcnId, machineTags, 0)
	s.setupSecurityListExpectations(c, vcnId, machineTags, 0)
	s.setupInternetGatewaysExpectations(c, vcnId, machineTags, 0)
	s.setupListRouteTableExpectations(c, vcnId, machineTags, 0)
	s.setupListSubnetsExpectations(c, vcnId, "fakeRouteTableId", machineTags, 0)

	ctx := envtesting.BootstrapTestContext(c)
	_, err := s.env.Bootstrap(ctx, environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		AvailableTools:          makeToolsList("centos"),
		BootstrapBase:           base.MustParseBaseFromString("ubuntu@22.04"),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorMatches, "no matching agent binaries available")

}

func (s *environSuite) setupDeleteSecurityListExpectations(c *tc.C, seclistId string, times int) {
	request := ociCore.DeleteSecurityListRequest{
		SecurityListId: makeStringPointer(seclistId),
	}

	response := ociCore.DeleteSecurityListResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}

	expect := s.fw.EXPECT().DeleteSecurityList(gomock.Any(), request).Return(response, nil)
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

	s.fw.EXPECT().GetSecurityList(gomock.Any(), requestGet).Return(responseGet, nil).AnyTimes()

}

func (s *environSuite) setupDeleteSubnetExpectations(c *tc.C, subnetIds []string) {
	for _, id := range subnetIds {
		request := ociCore.DeleteSubnetRequest{
			SubnetId: makeStringPointer(id),
		}

		response := ociCore.DeleteSubnetResponse{
			RawResponse: &http.Response{
				StatusCode: 201,
			},
		}
		s.netw.EXPECT().DeleteSubnet(gomock.Any(), request).Return(response, nil).AnyTimes()

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

		s.netw.EXPECT().GetSubnet(gomock.Any(), requestGet).Return(responseGet, nil).AnyTimes()
	}
}

func (s *environSuite) setupDeleteRouteTableExpectations(c *tc.C, vcnId, routeTableId string, t map[string]string) {
	s.setupListRouteTableExpectations(c, vcnId, t, 1)
	request := ociCore.DeleteRouteTableRequest{
		RtId: makeStringPointer(routeTableId),
	}

	response := ociCore.DeleteRouteTableResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	s.netw.EXPECT().DeleteRouteTable(gomock.Any(), request).Return(response, nil).AnyTimes()

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

	s.netw.EXPECT().GetRouteTable(gomock.Any(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (s *environSuite) setupDeleteInternetGatewayExpectations(c *tc.C, vcnId, igId string, t map[string]string) {
	s.setupInternetGatewaysExpectations(c, vcnId, t, 1)
	request := ociCore.DeleteInternetGatewayRequest{
		IgId: &igId,
	}

	response := ociCore.DeleteInternetGatewayResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	s.netw.EXPECT().DeleteInternetGateway(gomock.Any(), request).Return(response, nil)

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

	s.netw.EXPECT().GetInternetGateway(gomock.Any(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (s *environSuite) setupDeleteVcnExpectations(c *tc.C, vcnId string) {
	request := ociCore.DeleteVcnRequest{
		VcnId: &vcnId,
	}

	response := ociCore.DeleteVcnResponse{
		RawResponse: &http.Response{
			StatusCode: 201,
		},
	}
	s.netw.EXPECT().DeleteVcn(gomock.Any(), request).Return(response, nil)

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

	s.netw.EXPECT().GetVcn(gomock.Any(), requestGet).Return(responseGet, nil).AnyTimes()
}

func (s *environSuite) setupDeleteVolumesExpectations(c *tc.C) {
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

	s.storage.EXPECT().ListVolumes(gomock.Any(), listRequest.CompartmentId).Return(listResponse.Items, nil).AnyTimes()
	s.storage.EXPECT().GetVolume(gomock.Any(), requestVolume1).Return(responseVolume1, nil).AnyTimes()
	s.storage.EXPECT().GetVolume(gomock.Any(), requestVolume2).Return(responseVolume2, nil).AnyTimes()
}

func (s *environSuite) TestDestroyController(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	machineTags := map[string]string{
		tags.JujuController: testing.ControllerTag.Id(),
		tags.JujuModel:      testing.ModelTag.Id(),
	}

	vcnId := "fakeVCNId"
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateRunning, 1)
	s.setupStopInstanceExpectations(c,
		[]instanceTermination{
			{
				instanceId: s.testInstanceID,
				err:        nil,
			},
		},
	)
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateTerminated, 0)
	s.setupVcnExpectations(c, vcnId, machineTags, 1)
	s.setupListSubnetsExpectations(c, vcnId, "fakeRouteTableId", machineTags, 1)
	s.setupSecurityListExpectations(c, vcnId, machineTags, 1)
	s.setupDeleteRouteTableExpectations(c, vcnId, "fakeRouteTableId", machineTags)
	s.setupDeleteSubnetExpectations(c, []string{"fakeSubnetId1", "fakeSubnetId2", "fakeSubnetId3"})
	s.setupDeleteSecurityListExpectations(c, "fakeSecList", 0)
	s.setupDeleteInternetGatewayExpectations(c, vcnId, "fakeGwId", machineTags)
	s.setupDeleteVcnExpectations(c, vcnId)
	s.setupDeleteVolumesExpectations(c)

	err := s.env.DestroyController(c.Context(), s.controllerUUID)
	c.Assert(err, tc.IsNil)
}

func (s *environSuite) TestEnsureShapeConfig(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	machineTags := map[string]string{
		tags.JujuController: testing.ControllerTag.Id(),
		tags.JujuModel:      testing.ModelTag.Id(),
	}

	vcnId := "fakeVCNId"
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateRunning, 1)
	s.setupStopInstanceExpectations(c,
		[]instanceTermination{
			{
				instanceId: s.testInstanceID,
				err:        nil,
			},
		},
	)
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateTerminated, 0)
	s.setupVcnExpectations(c, vcnId, machineTags, 1)
	s.setupListSubnetsExpectations(c, vcnId, "fakeRouteTableId", machineTags, 1)
	s.setupSecurityListExpectations(c, vcnId, machineTags, 1)
	s.setupDeleteRouteTableExpectations(c, vcnId, "fakeRouteTableId", machineTags)
	s.setupDeleteSubnetExpectations(c, []string{"fakeSubnetId1", "fakeSubnetId2", "fakeSubnetId3"})
	s.setupDeleteSecurityListExpectations(c, "fakeSecList", 0)
	s.setupDeleteInternetGatewayExpectations(c, vcnId, "fakeGwId", machineTags)
	s.setupDeleteVcnExpectations(c, vcnId)
	s.setupDeleteVolumesExpectations(c)

	err := s.env.DestroyController(c.Context(), s.controllerUUID)
	c.Assert(err, tc.IsNil)
}
