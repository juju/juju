package oci_test

import (
	gomock "github.com/golang/mock/gomock"
	ocitesting "github.com/juju/juju/provider/oci/testing"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	ociCore "github.com/oracle/oci-go-sdk/core"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/oci"
)

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

func makeListsubnetsRequestResponse(subnetDetails []ociCore.Subnet) (ociCore.ListSubnetsRequest, ociCore.ListSubnetsResponse) {
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

type commonSuite struct {
	jujutesting.BaseSuite

	testInstanceID  string
	testCompartment string
	controllerUUID  string

	ident   *ocitesting.MockOCIIdentityClient
	compute *ocitesting.MockOCIComputeClient
	netw    *ocitesting.MockOCINetworkingClient
	fw      *ocitesting.MockOCIFirewallClient
	storage *ocitesting.MockOCIStorageClient

	env      *oci.Environ
	provider environs.EnvironProvider
	spec     environs.CloudSpec
	// instance    instance.Instance
	ociInstance *ociCore.Instance
	tags        map[string]string
}

func (s *commonSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	oci.SetImageCache(&oci.ImageCache{})

	s.controllerUUID = "7c6ab91e-80ab-4ce1-ad80-fe8b57bea5c3"
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

	s.env = env.(*oci.Environ)

	s.ociInstance = newFakeOCIInstance(s.testInstanceID, s.testCompartment, ociCore.InstanceLifecycleStateRunning)
	s.tags = map[string]string{
		tags.JujuController: s.controllerUUID,
		tags.JujuModel:      config.UUID(),
	}
	s.ociInstance.FreeformTags = s.tags
}

func (s *commonSuite) patchEnv(ctrl *gomock.Controller) {
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
}
