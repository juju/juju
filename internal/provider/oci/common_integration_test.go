// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	ociIdentity "github.com/oracle/oci-go-sdk/v65/identity"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/provider/oci"
	ocitesting "github.com/juju/juju/internal/provider/oci/testing"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
)

var clk = testclock.NewClock(time.Time{})

var advancingClock = testclock.AutoAdvancingClock{clk, clk.Advance}

func makeToolsList(series string) tools.List {
	var toolsVersion semversion.Binary
	toolsVersion.Number = semversion.MustParse("2.4.0")
	toolsVersion.Arch = arch.AMD64
	toolsVersion.Release = series
	return tools.List{{
		Version: toolsVersion,
		URL:     fmt.Sprintf("http://example.com/tools/juju-%s.tgz", toolsVersion),
		SHA256:  "1234567890abcdef",
		Size:    1024,
	}}
}

func makeListSecurityListsRequestResponse(seclistDetails []ociCore.SecurityList) (ociCore.ListSecurityListsRequest, ociCore.ListSecurityListsResponse) {
	if len(seclistDetails) == 0 {
		return ociCore.ListSecurityListsRequest{}, ociCore.ListSecurityListsResponse{}
	}

	compartment := seclistDetails[0].CompartmentId
	vcnId := seclistDetails[0].VcnId
	request := ociCore.ListSecurityListsRequest{
		CompartmentId: compartment,
		VcnId:         vcnId,
	}
	response := ociCore.ListSecurityListsResponse{
		Items: seclistDetails,
	}

	return request, response
}

func makeListInternetGatewaysRequestResponse(gwDetails []ociCore.InternetGateway) (ociCore.ListInternetGatewaysRequest, ociCore.ListInternetGatewaysResponse) {
	if len(gwDetails) == 0 {
		return ociCore.ListInternetGatewaysRequest{}, ociCore.ListInternetGatewaysResponse{}
	}

	compartment := gwDetails[0].CompartmentId
	vcnId := gwDetails[0].VcnId
	request := ociCore.ListInternetGatewaysRequest{
		CompartmentId: compartment,
		VcnId:         vcnId,
	}
	response := ociCore.ListInternetGatewaysResponse{
		Items: gwDetails,
	}
	return request, response
}

func makeListRouteTableRequestResponse(routeDetails []ociCore.RouteTable) (ociCore.ListRouteTablesRequest, ociCore.ListRouteTablesResponse) {
	if len(routeDetails) == 0 {
		return ociCore.ListRouteTablesRequest{}, ociCore.ListRouteTablesResponse{}
	}

	compartment := routeDetails[0].CompartmentId
	vcnId := routeDetails[0].VcnId

	request := ociCore.ListRouteTablesRequest{
		CompartmentId: compartment,
		VcnId:         vcnId,
	}
	response := ociCore.ListRouteTablesResponse{
		Items: routeDetails,
	}
	return request, response
}

func makeListInstancesRequestResponse(instances []ociCore.Instance) (ociCore.ListInstancesRequest, ociCore.ListInstancesResponse) {

	if len(instances) == 0 {
		return ociCore.ListInstancesRequest{}, ociCore.ListInstancesResponse{}
	}

	compartment := instances[0].CompartmentId

	request := ociCore.ListInstancesRequest{
		CompartmentId: compartment,
	}

	response := ociCore.ListInstancesResponse{
		Items: instances,
	}

	return request, response
}

func makeListAvailabilityDomainsRequestResponse(azDetails []ociIdentity.AvailabilityDomain) (ociIdentity.ListAvailabilityDomainsRequest, ociIdentity.ListAvailabilityDomainsResponse) {
	if len(azDetails) == 0 {
		return ociIdentity.ListAvailabilityDomainsRequest{}, ociIdentity.ListAvailabilityDomainsResponse{}
	}

	compartment := azDetails[0].CompartmentId
	request := ociIdentity.ListAvailabilityDomainsRequest{
		CompartmentId: compartment,
	}

	response := ociIdentity.ListAvailabilityDomainsResponse{
		Items: azDetails,
	}
	return request, response
}

func makeGetVnicRequestResponse(vnicDetails []ociCore.GetVnicResponse) ([]ociCore.GetVnicRequest, []ociCore.GetVnicResponse) {
	requests := make([]ociCore.GetVnicRequest, len(vnicDetails))
	responses := make([]ociCore.GetVnicResponse, len(vnicDetails))

	for idx, val := range vnicDetails {
		requests[idx] = ociCore.GetVnicRequest{
			VnicId: val.Id,
		}

		responses[idx] = val
	}

	return requests, responses
}

func newFakeOCIInstance(id, compartment string, state ociCore.InstanceLifecycleStateEnum) *ociCore.Instance {
	return &ociCore.Instance{
		AvailabilityDomain: new("fake-az"),
		Id:                 &id,
		CompartmentId:      &compartment,
		Region:             new("us-phoenix-1"),
		Shape:              new("VM.Standard1.1"),
		DisplayName:        new("fakeName"),
		FreeformTags:       map[string]string{},
		LifecycleState:     state,
	}
}

func makeGetInstanceRequestResponse(instanceDetails ociCore.Instance) (ociCore.GetInstanceRequest, ociCore.GetInstanceResponse) {
	request := ociCore.GetInstanceRequest{
		InstanceId: instanceDetails.Id,
	}

	response := ociCore.GetInstanceResponse{
		Instance: instanceDetails,
	}

	return request, response
}

func makeListVnicAttachmentsRequestResponse(vnicAttachDetails []ociCore.VnicAttachment) (ociCore.ListVnicAttachmentsRequest, ociCore.ListVnicAttachmentsResponse) {
	if len(vnicAttachDetails) == 0 {
		return ociCore.ListVnicAttachmentsRequest{}, ociCore.ListVnicAttachmentsResponse{}
	}
	compartment := vnicAttachDetails[0].CompartmentId
	instanceID := vnicAttachDetails[0].InstanceId

	request := ociCore.ListVnicAttachmentsRequest{
		CompartmentId: compartment,
		InstanceId:    instanceID,
	}

	response := ociCore.ListVnicAttachmentsResponse{
		Items: vnicAttachDetails,
	}
	return request, response
}

func listShapesResponse() []ociCore.Shape {
	return []ociCore.Shape{
		{
			Shape:                    new("VM.Standard1.1"),
			ProcessorDescription:     new("2.0 GHz Intel® Xeon® Platinum 8167M (Skylake)"),
			Ocpus:                    new(float32(1)),
			MemoryInGBs:              new(float32(7)),
			LocalDisks:               new(0),
			LocalDisksTotalSizeInGBs: (*float32)(nil),
			PlatformConfigOptions: &ociCore.ShapePlatformConfigOptions{
				Type: "INTEL_VM",
			},
			IsBilledForStoppedInstance: new(false),
			BillingType:                "PAID",
		},
		{
			Shape:                      new("VM.GPU.A10.1"),
			ProcessorDescription:       new("2.6 GHz Intel® Xeon® Platinum 8358 (Ice Lake)"),
			Ocpus:                      new(float32(15)),
			MemoryInGBs:                new(float32(240)),
			Gpus:                       new(1),
			GpuDescription:             new("NVIDIA® A10"),
			LocalDisks:                 new(0),
			LocalDisksTotalSizeInGBs:   (*float32)(nil),
			PlatformConfigOptions:      (*ociCore.ShapePlatformConfigOptions)(nil),
			IsBilledForStoppedInstance: new(false),
			BillingType:                "PAID",
		},
		{
			Shape:                      new("BM.Standard.A1.160"),
			ProcessorDescription:       new("3.0 GHz Ampere® Altra™"),
			Ocpus:                      new(float32(160)),
			MemoryInGBs:                new(float32(1024)),
			LocalDisks:                 new(0),
			LocalDisksTotalSizeInGBs:   (*float32)(nil),
			PlatformConfigOptions:      (*ociCore.ShapePlatformConfigOptions)(nil),
			IsBilledForStoppedInstance: new(false),
			BillingType:                "PAID",
		},
		{
			Shape:                new("VM.Standard.A1.Flex"),
			ProcessorDescription: new("3.0 GHz Ampere® Altra™"),
			OcpuOptions: &ociCore.ShapeOcpuOptions{
				Max: new(float32(80)),
			},
			MemoryOptions: &ociCore.ShapeMemoryOptions{
				MaxInGBs: new(float32(512)),
			},
			Ocpus:                      new(float32(1)),
			MemoryInGBs:                new(float32(6)),
			LocalDisks:                 new(0),
			LocalDisksTotalSizeInGBs:   (*float32)(nil),
			PlatformConfigOptions:      (*ociCore.ShapePlatformConfigOptions)(nil),
			IsBilledForStoppedInstance: new(false),
			BillingType:                "LIMITED_FREE",
		},
		{
			Shape:                new("VM.Standard3.Flex"),
			ProcessorDescription: new("2.0 GHz Intel® Xeon® Platinum 8167M (Skylake)"),
			OcpuOptions: &ociCore.ShapeOcpuOptions{
				Max: new(float32(32)),
			},
			MemoryOptions: &ociCore.ShapeMemoryOptions{
				MaxInGBs: new(float32(512)),
			},
			Ocpus:                      new(float32(1)),
			MemoryInGBs:                new(float32(6)),
			LocalDisks:                 new(0),
			LocalDisksTotalSizeInGBs:   (*float32)(nil),
			PlatformConfigOptions:      (*ociCore.ShapePlatformConfigOptions)(nil),
			IsBilledForStoppedInstance: new(false),
			BillingType:                "LIMITED_FREE",
		},
	}
}

type commonSuite struct {
	testhelpers.IsolationSuite

	testInstanceID  string
	testCompartment string
	controllerUUID  string

	ident   *ocitesting.MockIdentityClient
	compute *ocitesting.MockComputeClient
	netw    *ocitesting.MockNetworkingClient
	fw      *ocitesting.MockFirewallClient
	storage *ocitesting.MockStorageClient

	env         *oci.Environ
	provider    environs.EnvironProvider
	spec        environscloudspec.CloudSpec
	config      *config.Config
	ociInstance *ociCore.Instance
	tags        map[string]string
	ctrlTags    map[string]string
}

func (s *commonSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	oci.SetImageCache(&oci.ImageCache{})

	s.controllerUUID = jujutesting.ControllerTag.Id()
	s.testInstanceID = "ocid1.instance.oc1.phx.abyhqljt4bl4i76iforb7wczlc4qmmmrmzvngeyzi2n45bxsee7a11rukjla"
	s.testCompartment = "ocid1.compartment.oc1..aaaaaaaaakr75vvb5yx4nkm7ag7ekvluap7afa2y4zprswuprcnehqecwqga"

	s.provider = &oci.EnvironProvider{}
	s.spec = fakeCloudSpec()

	config := newConfig(c, jujutesting.Attrs{"compartment-id": s.testCompartment})
	env, err := environs.Open(c.Context(), s.provider, environs.OpenParams{
		Cloud:          s.spec,
		Config:         config,
		ControllerUUID: s.controllerUUID,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env, tc.NotNil)

	s.config = config
	s.env = env.(*oci.Environ)
	s.env.SetClock(&advancingClock)

	s.ociInstance = newFakeOCIInstance(s.testInstanceID, s.testCompartment, ociCore.InstanceLifecycleStateRunning)
	s.tags = map[string]string{
		tags.JujuController: s.controllerUUID,
		tags.JujuModel:      config.UUID(),
	}
	s.ctrlTags = map[string]string{
		tags.JujuController:   s.controllerUUID,
		tags.JujuModel:        s.config.UUID(),
		tags.JujuIsController: "true",
	}
	s.ociInstance.FreeformTags = s.tags
}

func (e *commonSuite) setupListInstancesExpectations(c *tc.C, instanceId string, state ociCore.InstanceLifecycleStateEnum, times int) {
	listInstancesRequest, listInstancesResponse := makeListInstancesRequestResponse(
		[]ociCore.Instance{
			{
				AvailabilityDomain: new("fakeZone1"),
				CompartmentId:      &e.testCompartment,
				Id:                 new(instanceId),
				LifecycleState:     state,
				Region:             new("us-phoenix-1"),
				Shape:              new("VM.Standard1.1"),
				DisplayName:        new("fakeName"),
				FreeformTags:       e.tags,
			},
		},
	)
	expect := e.compute.EXPECT().ListInstances(gomock.Any(),
		listInstancesRequest.CompartmentId).Return(
		listInstancesResponse.Items, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *commonSuite) patchEnv(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.compute = ocitesting.NewMockComputeClient(ctrl)
	s.ident = ocitesting.NewMockIdentityClient(ctrl)
	s.netw = ocitesting.NewMockNetworkingClient(ctrl)
	s.fw = ocitesting.NewMockFirewallClient(ctrl)
	s.storage = ocitesting.NewMockStorageClient(ctrl)

	s.env.Compute = s.compute
	s.env.Networking = s.netw
	s.env.Firewall = s.fw
	s.env.Storage = s.storage
	s.env.Identity = s.ident

	return ctrl
}

func (s *commonSuite) setupEnsureNetworksExpectations(c *tc.C, vcnId string, machineTags map[string]string) {
	s.setupAvailabilityDomainsExpectations(c, 0)
	s.setupVcnExpectations(c, vcnId, machineTags, 1)
	s.setupSecurityListExpectations(c, vcnId, machineTags, 1)
	s.setupInternetGatewaysExpectations(c, vcnId, machineTags, 1)
	s.setupListRouteTableExpectations(c, vcnId, machineTags, 1)
	s.setupListSubnetsExpectations(c, vcnId, "fakeRouteTableId", machineTags, 1)
}

func (s *commonSuite) setupInternetGatewaysExpectations(c *tc.C, vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("%s-%s", oci.InternetGatewayPrefix, t[tags.JujuController])
	enabled := true
	request, response := makeListInternetGatewaysRequestResponse([]ociCore.InternetGateway{
		{
			CompartmentId: &s.testCompartment,
			Id:            new("fakeGwId"),
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

func (s *commonSuite) setupListRouteTableExpectations(c *tc.C, vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("%s-%s", oci.RouteTablePrefix, t[tags.JujuController])
	request, response := makeListRouteTableRequestResponse([]ociCore.RouteTable{
		{
			CompartmentId:  &s.testCompartment,
			Id:             new("fakeRouteTableId"),
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

func (s *commonSuite) setupListSubnetsExpectations(c *tc.C, vcnId, route string, t map[string]string, times int) {
	zone1 := "fakeZone1"
	zone2 := "fakeZone2"
	zone3 := "fakeZone3"
	displayNameZone1 := fmt.Sprintf("juju-%s-%s-%s", zone1, t[tags.JujuController], t[tags.JujuModel])
	displayNameZone2 := fmt.Sprintf("juju-%s-%s-%s", zone2, t[tags.JujuController], t[tags.JujuModel])
	displayNameZone3 := fmt.Sprintf("juju-%s-%s-%s", zone3, t[tags.JujuController], t[tags.JujuModel])
	response := []ociCore.Subnet{
		{
			AvailabilityDomain: &zone1,
			CidrBlock:          new(oci.DefaultAddressSpace),
			CompartmentId:      &s.testCompartment,
			Id:                 new("fakeSubnetId1"),
			VcnId:              &vcnId,
			DisplayName:        &displayNameZone1,
			RouteTableId:       &route,
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			FreeformTags:       t,
		},
		{
			AvailabilityDomain: &zone2,
			CidrBlock:          new(oci.DefaultAddressSpace),
			CompartmentId:      &s.testCompartment,
			Id:                 new("fakeSubnetId2"),
			VcnId:              &vcnId,
			DisplayName:        &displayNameZone2,
			RouteTableId:       &route,
			LifecycleState:     ociCore.SubnetLifecycleStateAvailable,
			FreeformTags:       t,
		},
		{
			AvailabilityDomain: &zone3,
			CidrBlock:          new(oci.DefaultAddressSpace),
			CompartmentId:      &s.testCompartment,
			Id:                 new("fakeSubnetId3"),
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

func (s *commonSuite) setupAvailabilityDomainsExpectations(c *tc.C, times int) {
	request, response := makeListAvailabilityDomainsRequestResponse([]ociIdentity.AvailabilityDomain{
		{
			Name:          new("fakeZone1"),
			CompartmentId: &s.testCompartment,
		},
		{
			Name:          new("fakeZone2"),
			CompartmentId: &s.testCompartment,
		},
		{
			Name:          new("fakeZone3"),
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

func (s *commonSuite) setupVcnExpectations(c *tc.C, vcnId string, t map[string]string, times int) {
	vcnName := makeVcnName(t[tags.JujuController], t[tags.JujuModel])
	vcnResponse := []ociCore.Vcn{
		{
			CompartmentId:         &s.testCompartment,
			CidrBlock:             new(oci.DefaultAddressSpace),
			Id:                    &vcnId,
			LifecycleState:        ociCore.VcnLifecycleStateAvailable,
			DefaultRouteTableId:   new("fakeRouteTable"),
			DefaultSecurityListId: new("fakeSeclist"),
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

func (s *commonSuite) setupSecurityListExpectations(c *tc.C, vcnId string, t map[string]string, times int) {
	name := fmt.Sprintf("juju-seclist-%s-%s", t[tags.JujuController], t[tags.JujuModel])
	request, response := makeListSecurityListsRequestResponse([]ociCore.SecurityList{
		{
			CompartmentId: &s.testCompartment,
			VcnId:         &vcnId,
			Id:            new("fakeSecList"),
			DisplayName:   &name,
			FreeformTags:  t,
			EgressSecurityRules: []ociCore.EgressSecurityRule{
				{
					Destination: new(oci.AllowAllPrefix),
					Protocol:    new(oci.AllProtocols),
				},
			},
			IngressSecurityRules: []ociCore.IngressSecurityRule{
				{
					Source:   new(oci.AllowAllPrefix),
					Protocol: new(oci.AllProtocols),
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
