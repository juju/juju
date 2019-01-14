// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	gitjujutesting "github.com/juju/testing"
	"github.com/juju/utils/arch"
	"github.com/juju/version"

	gomock "github.com/golang/mock/gomock"
	ocitesting "github.com/juju/juju/provider/oci/testing"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	ociCore "github.com/oracle/oci-go-sdk/core"
	ociIdentity "github.com/oracle/oci-go-sdk/identity"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/oci"
	"github.com/juju/juju/tools"
)

var clk = testclock.NewClock(time.Time{})
var advancingClock = testclock.AutoAdvancingClock{clk, clk.Advance}

func makeToolsList(series string) tools.List {
	var toolsVersion version.Binary
	toolsVersion.Number = version.MustParse("2.4.0")
	toolsVersion.Arch = arch.AMD64
	toolsVersion.Series = series
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

func makeListVcnRequestResponse(vcnDetails []ociCore.Vcn) (ociCore.ListVcnsRequest, ociCore.ListVcnsResponse) {
	if len(vcnDetails) == 0 {
		return ociCore.ListVcnsRequest{}, ociCore.ListVcnsResponse{}
	}
	compartment := vcnDetails[0].CompartmentId
	request := ociCore.ListVcnsRequest{
		CompartmentId: compartment,
	}

	response := ociCore.ListVcnsResponse{
		Items: vcnDetails,
	}
	return request, response
}

func makeListSubnetsRequestResponse(subnetDetails []ociCore.Subnet) (ociCore.ListSubnetsRequest, ociCore.ListSubnetsResponse) {
	if len(subnetDetails) == 0 {
		return ociCore.ListSubnetsRequest{}, ociCore.ListSubnetsResponse{}
	}
	compartment := subnetDetails[0].CompartmentId
	vcnID := subnetDetails[0].VcnId

	request := ociCore.ListSubnetsRequest{
		CompartmentId: compartment,
		VcnId:         vcnID,
	}

	response := ociCore.ListSubnetsResponse{
		Items: subnetDetails,
	}
	return request, response
}

func newFakeOCIInstance(Id, compartment string, state ociCore.InstanceLifecycleStateEnum) *ociCore.Instance {
	return &ociCore.Instance{
		AvailabilityDomain: makeStringPointer("fake-az"),
		Id:                 &Id,
		CompartmentId:      &compartment,
		Region:             makeStringPointer("us-phoenix-1"),
		Shape:              makeStringPointer("VM.Standard1.1"),
		DisplayName:        makeStringPointer("fakeName"),
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

func makeShapesRequestResponse(compartment, id string, shapeNames []string) (ociCore.ListShapesRequest, ociCore.ListShapesResponse) {
	shapesRequest := ociCore.ListShapesRequest{
		CompartmentId: &compartment,
		ImageId:       &id,
	}

	ociShapes := []ociCore.Shape{}
	for _, val := range shapeNames {
		shape := ociCore.Shape{
			Shape: makeStringPointer(val),
		}
		ociShapes = append(ociShapes, shape)
	}

	shapesResponse := ociCore.ListShapesResponse{
		Items: ociShapes,
	}

	return shapesRequest, shapesResponse
}

func makeListImageRequestResponse(imgDetails []ociCore.Image) (ociCore.ListImagesRequest, ociCore.ListImagesResponse) {
	if len(imgDetails) == 0 {
		return ociCore.ListImagesRequest{}, ociCore.ListImagesResponse{}
	}

	compartment := imgDetails[0].CompartmentId
	request := ociCore.ListImagesRequest{
		CompartmentId: compartment,
	}
	response := ociCore.ListImagesResponse{
		Items: imgDetails,
	}
	return request, response
}

type commonSuite struct {
	gitjujutesting.IsolationSuite

	testInstanceID  string
	testCompartment string
	controllerUUID  string

	ident   *ocitesting.MockOCIIdentityClient
	compute *ocitesting.MockOCIComputeClient
	netw    *ocitesting.MockOCINetworkingClient
	fw      *ocitesting.MockOCIFirewallClient
	storage *ocitesting.MockOCIStorageClient

	env         *oci.Environ
	provider    environs.EnvironProvider
	spec        environs.CloudSpec
	config      *config.Config
	ociInstance *ociCore.Instance
	tags        map[string]string
	ctrlTags    map[string]string
}

func (s *commonSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	oci.SetImageCache(&oci.ImageCache{})

	s.controllerUUID = jujutesting.ControllerTag.Id()
	s.testInstanceID = "ocid1.instance.oc1.phx.abyhqljt4bl4i76iforb7wczlc4qmmmrmzvngeyzi2n45bxsee7a11rukjla"
	s.testCompartment = "ocid1.compartment.oc1..aaaaaaaaakr75vvb5yx4nkm7ag7ekvluap7afa2y4zprswuprcnehqecwqga"

	s.provider = &oci.EnvironProvider{}
	s.spec = fakeCloudSpec()

	config := newConfig(c, jujutesting.Attrs{"compartment-id": s.testCompartment})
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: config,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)

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

func (e *commonSuite) setupListInstancesExpectations(instanceId string, state ociCore.InstanceLifecycleStateEnum, times int) {
	listInstancesRequest, listInstancesResponse := makeListInstancesRequestResponse(
		[]ociCore.Instance{
			{
				AvailabilityDomain: makeStringPointer("fakeZone1"),
				CompartmentId:      &e.testCompartment,
				Id:                 makeStringPointer(instanceId),
				LifecycleState:     state,
				Region:             makeStringPointer("us-phoenix-1"),
				Shape:              makeStringPointer("VM.Standard1.1"),
				DisplayName:        makeStringPointer("fakeName"),
				FreeformTags:       e.tags,
			},
		},
	)
	expect := e.compute.EXPECT().ListInstances(
		context.Background(), listInstancesRequest).Return(
		listInstancesResponse, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *commonSuite) patchEnv(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.compute = ocitesting.NewMockOCIComputeClient(ctrl)
	s.ident = ocitesting.NewMockOCIIdentityClient(ctrl)
	s.netw = ocitesting.NewMockOCINetworkingClient(ctrl)
	s.fw = ocitesting.NewMockOCIFirewallClient(ctrl)
	s.storage = ocitesting.NewMockOCIStorageClient(ctrl)

	s.env.Compute = s.compute
	s.env.Networking = s.netw
	s.env.Firewall = s.fw
	s.env.Storage = s.storage
	s.env.Identity = s.ident

	return ctrl
}
