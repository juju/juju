// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	ctx "context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	ociCommon "github.com/oracle/oci-go-sdk/v65/common"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/provider/oci/testing"
)

func TestComputeClientSuite(t *stdtesting.T) {
	tc.Run(t, &computeClientSuite{})
}

func TestNetworkClientSuite(t *stdtesting.T) {
	tc.Run(t, &networkClientSuite{})
}

func TestStorageClientSuite(t *stdtesting.T) {
	tc.Run(t, &storageClientSuite{})
}

var compartmentID = "compartment-id"

type computeClientSuite struct {
	client    *computeClient
	mockAPI   *testing.MockOCIComputeClient
	images    []ociCore.Image
	instances []ociCore.Instance
	vnics     []ociCore.VnicAttachment
	volumes   []ociCore.VolumeAttachment
}

func (s *computeClientSuite) SetUpSuite(c *tc.C) {
	s.images = []ociCore.Image{
		{
			CompartmentId:          &compartmentID,
			Id:                     new("fakeUbuntu1"),
			OperatingSystem:        new("Canonical Ubuntu"),
			OperatingSystemVersion: new("22.04"),
			DisplayName:            new("Canonical-Ubuntu-22.04-2018.01.11-0"),
		},
		{
			CompartmentId:          &compartmentID,
			Id:                     new("fakeUbuntu2"),
			OperatingSystem:        new("Canonical Ubuntu"),
			OperatingSystemVersion: new("22.04"),
			DisplayName:            new("Canonical-Ubuntu-22.04-2018.01.12-0"),
		},
		{
			CompartmentId:          &compartmentID,
			Id:                     new("fakeCentOS"),
			OperatingSystem:        new("CentOS"),
			OperatingSystemVersion: new("7"),
			DisplayName:            new("CentOS-7-2017.10.19-0"),
		},
	}

	s.instances = []ociCore.Instance{
		{
			AvailabilityDomain: new("fakeZone1"),
			CompartmentId:      &compartmentID,
			Id:                 new("instID"),
			LifecycleState:     ociCore.InstanceLifecycleStateRunning,
			Region:             new("us-phoenix-1"),
			Shape:              new("VM.Standard1.1"),
			DisplayName:        new("fakeName"),
		},
		{
			AvailabilityDomain: new("fakeZone2"),
			CompartmentId:      &compartmentID,
			Id:                 new("instID3"),
			LifecycleState:     ociCore.InstanceLifecycleStateRunning,
			Region:             new("us-phoenix-1"),
			Shape:              new("VM.Standard2.1"),
			DisplayName:        new("fakeNameTheSecond"),
		},
	}

	fakeInstID := "fakeInstanceId"
	s.vnics = []ociCore.VnicAttachment{
		{
			Id:                 new("fakeAttachmentId"),
			AvailabilityDomain: new("fake"),
			CompartmentId:      &compartmentID,
			InstanceId:         &fakeInstID,
			LifecycleState:     ociCore.VnicAttachmentLifecycleStateAttached,
			DisplayName:        new("fakeAttachmentName"),
			NicIndex:           new(0),
			VnicId:             new("vnicID1"),
		},
		{
			Id:                 new("fakeAttachmentId2"),
			AvailabilityDomain: new("fake2"),
			CompartmentId:      &compartmentID,
			InstanceId:         &fakeInstID,
			LifecycleState:     ociCore.VnicAttachmentLifecycleStateAttached,
			DisplayName:        new("fakeAttachmentName2"),
			NicIndex:           new(1),
			VnicId:             new("vnicID2"),
		},
	}

	s.volumes = []ociCore.VolumeAttachment{
		ociCore.IScsiVolumeAttachment{
			AvailabilityDomain: new("fakeZone1"),
			InstanceId:         &fakeInstID,
			CompartmentId:      &compartmentID,
			Iqn:                new("bogus"),
			Id:                 new("fakeVolumeAttachment1"),
			VolumeId:           new("volume1"),
			Ipv4:               new("192.168.1.1"),
			DisplayName:        new("fakeVolumeAttachment"),
			ChapSecret:         new("superSecretPassword"),
			ChapUsername:       new("JohnDoe"),
			LifecycleState:     ociCore.VolumeAttachmentLifecycleStateAttached,
		},
		ociCore.IScsiVolumeAttachment{
			AvailabilityDomain: new("fakeZone1"),
			InstanceId:         &fakeInstID,
			CompartmentId:      &compartmentID,
			Iqn:                new("bogus"),
			Id:                 new("fakeVolumeAttachment2"),
			VolumeId:           new("volume2"),
			Ipv4:               new("192.168.1.42"),
			DisplayName:        new("fakeVolumeAttachment"),
			ChapSecret:         new("superSecretPassword"),
			ChapUsername:       new("JohnDoe"),
			LifecycleState:     ociCore.VolumeAttachmentLifecycleStateAttached,
		},
	}
}

func (s *computeClientSuite) TestListImages(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{
		Items: s.images,
	}, nil)

	obtained, err := s.client.ListImages(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.images)
}

func (s *computeClientSuite) TestListImagesPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{
		Items:       []ociCore.Image{s.images[0]},
		OpcNextPage: new("test-pagination"),
	}, nil)
	s.mockAPI.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{
		Items:       []ociCore.Image{s.images[1]},
		OpcNextPage: new("test-pagination"),
	}, nil)
	s.mockAPI.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{
		Items: []ociCore.Image{s.images[2]},
	}, nil)

	obtained, err := s.client.ListImages(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.images)
}

func (s *computeClientSuite) TestListImagesFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListImages(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListImagesFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{
		Items:       []ociCore.Image{s.images[0]},
		OpcNextPage: new("test-pagination"),
	}, nil)
	s.mockAPI.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListImages(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListShapes(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	shape1 := "VM.Standard2.1"
	shape2 := "VM.Standard1.2"
	req := ociCore.ListShapesRequest{
		CompartmentId:      &compartmentID,
		AvailabilityDomain: nil,
		Limit:              nil,
		Page:               nil,
		ImageId:            s.images[1].Id,
		OpcRequestId:       nil,
		RequestMetadata:    ociCommon.RequestMetadata{},
	}
	resp := ociCore.ListShapesResponse{
		Items: []ociCore.Shape{
			{Shape: &shape1},
			{Shape: &shape2},
		},
	}
	s.mockAPI.EXPECT().ListShapes(gomock.Any(), req).Return(resp, nil)

	obtained, err := s.client.ListShapes(ctx.TODO(), &compartmentID, s.images[1].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 2)
	c.Assert(obtained[0].Shape, tc.Equals, &shape1)
	c.Assert(obtained[1].Shape, tc.Equals, &shape2)
}

func (s *computeClientSuite) TestListShapesPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	shape1 := "VM.Standard2.1"
	shape2 := "VM.Standard1.2"
	req := ociCore.ListShapesRequest{
		CompartmentId: &compartmentID,
		ImageId:       s.images[1].Id,
	}
	shapes := []ociCore.Shape{
		{Shape: &shape1},
		{Shape: &shape2},
	}
	resp := ociCore.ListShapesResponse{
		Items:       []ociCore.Shape{shapes[0]},
		OpcNextPage: new("test-pagination"),
	}

	s.mockAPI.EXPECT().ListShapes(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListShapes(gomock.Any(), req).Return(ociCore.ListShapesResponse{
		Items: []ociCore.Shape{shapes[1]},
	}, nil)

	obtained, err := s.client.ListShapes(ctx.TODO(), &compartmentID, req.ImageId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.HasLen, 2)
	c.Assert(obtained[0].Shape, tc.Equals, &shape1)
	c.Assert(obtained[1].Shape, tc.Equals, &shape2)
}

func (s *computeClientSuite) TestListShapesFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListShapes(gomock.Any(), gomock.Any()).Return(ociCore.ListShapesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListShapes(ctx.TODO(), &compartmentID, new("testFail"))
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListShapesFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	shape1 := "VM.Standard2.1"

	req := ociCore.ListShapesRequest{
		CompartmentId: &compartmentID,
		ImageId:       new("testFail"),
	}
	shapes := []ociCore.Shape{
		{Shape: &shape1},
	}
	resp := ociCore.ListShapesResponse{
		Items:       shapes,
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListShapes(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListShapes(gomock.Any(), req).Return(
		ociCore.ListShapesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListShapes(ctx.TODO(), &compartmentID, req.ImageId)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListInstances(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Return(ociCore.ListInstancesResponse{
		Items: s.instances,
	}, nil)

	obtained, err := s.client.ListInstances(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.instances)
}

func (s *computeClientSuite) TestListInstancesPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Return(ociCore.ListInstancesResponse{
		Items:       []ociCore.Instance{s.instances[0]},
		OpcNextPage: new("test-pagination"),
	}, nil)
	s.mockAPI.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Return(ociCore.ListInstancesResponse{
		Items: []ociCore.Instance{s.instances[1]},
	}, nil)

	obtained, err := s.client.ListInstances(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.instances)
}

func (s *computeClientSuite) TestListInstancesFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Return(ociCore.ListInstancesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListInstances(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListInstancesFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Return(ociCore.ListInstancesResponse{
		Items:       []ociCore.Instance{s.instances[0]},
		OpcNextPage: new("test-pagination"),
	}, nil)
	s.mockAPI.EXPECT().ListInstances(gomock.Any(), gomock.Any()).Return(ociCore.ListInstancesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListInstances(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListVnicAttachments(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListVnicAttachments(gomock.Any(), gomock.Any()).Return(ociCore.ListVnicAttachmentsResponse{
		Items: s.vnics,
	}, nil)

	obtained, err := s.client.ListVnicAttachments(ctx.TODO(), &compartmentID, s.vnics[0].InstanceId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.vnics)
}

func (s *computeClientSuite) TestListVnicAttachmentsPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListVnicAttachmentsRequest{
		CompartmentId: &compartmentID,
		InstanceId:    s.vnics[0].InstanceId,
	}
	resp := ociCore.ListVnicAttachmentsResponse{
		Items:       []ociCore.VnicAttachment{s.vnics[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListVnicAttachments(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListVnicAttachments(gomock.Any(), req).Return(
		ociCore.ListVnicAttachmentsResponse{
			Items: []ociCore.VnicAttachment{s.vnics[1]},
		}, nil)

	obtained, err := s.client.ListVnicAttachments(ctx.TODO(), &compartmentID, s.vnics[0].InstanceId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.vnics)
}

func (s *computeClientSuite) TestListVnicAttachmentsFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListVnicAttachments(gomock.Any(), gomock.Any()).Return(ociCore.ListVnicAttachmentsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListVnicAttachments(ctx.TODO(), &compartmentID, s.vnics[0].InstanceId)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListVnicAttachmentsFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListVnicAttachmentsRequest{
		CompartmentId: &compartmentID,
		InstanceId:    s.vnics[0].InstanceId,
	}
	resp := ociCore.ListVnicAttachmentsResponse{
		Items:       []ociCore.VnicAttachment{s.vnics[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListVnicAttachments(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListVnicAttachments(gomock.Any(), req).Return(ociCore.ListVnicAttachmentsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListVnicAttachments(ctx.TODO(), &compartmentID, s.vnics[0].InstanceId)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListVolumeAttachments(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListVolumeAttachmentsRequest{
		CompartmentId: &compartmentID,
		InstanceId:    s.volumes[0].GetInstanceId(),
	}
	resp := ociCore.ListVolumeAttachmentsResponse{
		Items: s.volumes,
	}
	s.mockAPI.EXPECT().ListVolumeAttachments(gomock.Any(), req).Return(resp, nil)

	obtained, err := s.client.ListVolumeAttachments(ctx.TODO(), &compartmentID, s.volumes[0].GetInstanceId())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.volumes)
}

func (s *computeClientSuite) TestListVolumeAttachmentsPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListVolumeAttachmentsRequest{
		CompartmentId: &compartmentID,
		InstanceId:    s.volumes[0].GetInstanceId(),
	}
	resp := ociCore.ListVolumeAttachmentsResponse{
		Items:       []ociCore.VolumeAttachment{s.volumes[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListVolumeAttachments(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListVolumeAttachments(gomock.Any(), req).Return(
		ociCore.ListVolumeAttachmentsResponse{
			Items: []ociCore.VolumeAttachment{s.volumes[1]},
		}, nil)

	obtained, err := s.client.ListVolumeAttachments(ctx.TODO(), &compartmentID, s.volumes[0].GetInstanceId())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.volumes)
}

func (s *computeClientSuite) TestListVolumeAttachmentsFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListVolumeAttachments(gomock.Any(), gomock.Any()).Return(ociCore.ListVolumeAttachmentsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListVolumeAttachments(ctx.TODO(), &compartmentID, s.volumes[0].GetInstanceId())
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) TestListVolumeAttachmentsFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListVolumeAttachmentsRequest{
		CompartmentId: &compartmentID,
		InstanceId:    s.volumes[0].GetInstanceId(),
	}
	resp := ociCore.ListVolumeAttachmentsResponse{
		Items:       []ociCore.VolumeAttachment{s.volumes[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListVolumeAttachments(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListVolumeAttachments(gomock.Any(), req).Return(ociCore.ListVolumeAttachmentsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListVolumeAttachments(ctx.TODO(), &compartmentID, s.volumes[0].GetInstanceId())
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *computeClientSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockAPI = testing.NewMockOCIComputeClient(ctrl)
	s.client = &computeClient{s.mockAPI}
	return ctrl
}

type networkClientSuite struct {
	client        *networkClient
	mockAPI       *testing.MockOCIVirtualNetworkingClient
	vcns          []ociCore.Vcn
	subnets       []ociCore.Subnet
	gateways      []ociCore.InternetGateway
	tables        []ociCore.RouteTable
	securityLists []ociCore.SecurityList
}

func (s *networkClientSuite) SetUpSuite(c *tc.C) {
	s.vcns = []ociCore.Vcn{
		{
			CompartmentId: &compartmentID,
			Id:            new("idOne"),
		},
		{
			CompartmentId: &compartmentID,
			Id:            new("idTwo"),
		},
	}
	s.subnets = []ociCore.Subnet{
		{
			CompartmentId: &compartmentID,
			Id:            new("fakeSubnetId"),
			VcnId:         s.vcns[0].Id,
		}, {
			CompartmentId: &compartmentID,
			Id:            new("fakeSubnetId2"),
			VcnId:         s.vcns[0].Id,
		},
	}
	s.gateways = []ociCore.InternetGateway{
		{
			CompartmentId: &compartmentID,
			Id:            new("fakeGwId"),
			VcnId:         s.vcns[0].Id,
		},
		{
			CompartmentId: &compartmentID,
			Id:            new("fakeGwId2"),
			VcnId:         s.vcns[0].Id,
		},
	}
	s.tables = []ociCore.RouteTable{
		{
			CompartmentId: &compartmentID,
			Id:            new("fakeRouteTableId"),
			VcnId:         s.vcns[0].Id,
		},
		{
			CompartmentId: &compartmentID,
			Id:            new("fakeRouteTableId2"),
			VcnId:         s.vcns[0].Id,
		},
	}
	s.securityLists = []ociCore.SecurityList{
		{
			CompartmentId: &compartmentID,
			VcnId:         s.vcns[0].Id,
			Id:            new("fakeSecList"),
			EgressSecurityRules: []ociCore.EgressSecurityRule{
				{
					Destination: new("dst"),
				},
			},
			IngressSecurityRules: []ociCore.IngressSecurityRule{
				{
					Source: new("src"),
				},
			},
		},
		{
			CompartmentId: &compartmentID,
			VcnId:         s.vcns[0].Id,
			Id:            new("fakeSecList3"),
			EgressSecurityRules: []ociCore.EgressSecurityRule{
				{
					Destination: new("dst"),
				},
			},
			IngressSecurityRules: []ociCore.IngressSecurityRule{
				{
					Source: new("src"),
				},
			},
		},
	}
}

func (s *networkClientSuite) TestListVcns(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListVcns(gomock.Any(), gomock.Any()).Return(ociCore.ListVcnsResponse{
		Items: s.vcns,
	}, nil)

	obtained, err := s.client.ListVcns(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.vcns)
}

func (s *networkClientSuite) TestListVcnsPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListVcnsRequest{
		CompartmentId: &compartmentID,
	}
	resp := ociCore.ListVcnsResponse{
		Items:       []ociCore.Vcn{s.vcns[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListVcns(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListVcns(gomock.Any(), req).Return(ociCore.ListVcnsResponse{
		Items: []ociCore.Vcn{s.vcns[1]},
	}, nil)

	obtained, err := s.client.ListVcns(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.vcns)
}

func (s *networkClientSuite) TestListVcnsFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListVcns(gomock.Any(), gomock.Any()).Return(ociCore.ListVcnsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListVcns(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListVcnsFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListVcnsRequest{
		CompartmentId: &compartmentID,
	}
	resp := ociCore.ListVcnsResponse{
		Items:       s.vcns,
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListVcns(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListVcns(gomock.Any(), req).Return(ociCore.ListVcnsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListVcns(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListSubnets(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListSubnetsRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	s.mockAPI.EXPECT().ListSubnets(gomock.Any(), req).Return(ociCore.ListSubnetsResponse{
		Items: s.subnets,
	}, nil)

	obtained, err := s.client.ListSubnets(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.subnets)
}

func (s *networkClientSuite) TestListSubnetsPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListSubnetsRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	resp := ociCore.ListSubnetsResponse{
		Items:       []ociCore.Subnet{s.subnets[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListSubnets(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListSubnets(gomock.Any(), req).Return(ociCore.ListSubnetsResponse{
		Items: []ociCore.Subnet{s.subnets[1]},
	}, nil)

	obtained, err := s.client.ListSubnets(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.subnets)
}

func (s *networkClientSuite) TestListSubnetsFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListSubnetsRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	s.mockAPI.EXPECT().ListSubnets(gomock.Any(), req).Return(ociCore.ListSubnetsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListSubnets(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListSubnetsFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListSubnetsRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	resp := ociCore.ListSubnetsResponse{
		Items:       []ociCore.Subnet{s.subnets[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListSubnets(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListSubnets(gomock.Any(), req).Return(ociCore.ListSubnetsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListSubnets(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListInternetGateways(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListInternetGatewaysRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	s.mockAPI.EXPECT().ListInternetGateways(gomock.Any(), req).Return(ociCore.ListInternetGatewaysResponse{
		Items: s.gateways,
	}, nil)

	obtained, err := s.client.ListInternetGateways(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.gateways)
}

func (s *networkClientSuite) TestListInternetGatewaysPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListInternetGatewaysRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	resp := ociCore.ListInternetGatewaysResponse{
		Items:       []ociCore.InternetGateway{s.gateways[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListInternetGateways(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListInternetGateways(gomock.Any(), req).Return(ociCore.ListInternetGatewaysResponse{
		Items: []ociCore.InternetGateway{s.gateways[1]},
	}, nil)

	obtained, err := s.client.ListInternetGateways(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.gateways)
}

func (s *networkClientSuite) TestListInternetGatewaysFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListInternetGatewaysRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	s.mockAPI.EXPECT().ListInternetGateways(gomock.Any(), req).Return(ociCore.ListInternetGatewaysResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListInternetGateways(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListInternetGatewaysFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListInternetGatewaysRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	resp := ociCore.ListInternetGatewaysResponse{
		Items:       s.gateways,
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListInternetGateways(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListInternetGateways(gomock.Any(), req).Return(ociCore.ListInternetGatewaysResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListInternetGateways(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListRouteTables(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListRouteTablesRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	s.mockAPI.EXPECT().ListRouteTables(gomock.Any(), req).Return(ociCore.ListRouteTablesResponse{
		Items: s.tables,
	}, nil)

	obtained, err := s.client.ListRouteTables(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.tables)
}

func (s *networkClientSuite) TestListRouteTablesPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListRouteTablesRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	resp := ociCore.ListRouteTablesResponse{
		Items:       []ociCore.RouteTable{s.tables[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListRouteTables(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListRouteTables(gomock.Any(), req).Return(ociCore.ListRouteTablesResponse{
		Items: []ociCore.RouteTable{s.tables[1]},
	}, nil)

	obtained, err := s.client.ListRouteTables(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.tables)
}

func (s *networkClientSuite) TestListRouteTablesFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListRouteTablesRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	s.mockAPI.EXPECT().ListRouteTables(gomock.Any(), req).Return(ociCore.ListRouteTablesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListRouteTables(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListRouteTablesFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListRouteTablesRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	resp := ociCore.ListRouteTablesResponse{
		Items:       []ociCore.RouteTable{s.tables[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListRouteTables(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListRouteTables(gomock.Any(), req).Return(ociCore.ListRouteTablesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListRouteTables(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListSecurityLists(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListSecurityListsRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	s.mockAPI.EXPECT().ListSecurityLists(gomock.Any(), req).Return(ociCore.ListSecurityListsResponse{
		Items: s.securityLists,
	}, nil)

	obtained, err := s.client.ListSecurityLists(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.securityLists)
}

func (s *networkClientSuite) TestListSecurityListsPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListSecurityListsRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	resp := ociCore.ListSecurityListsResponse{
		Items:       []ociCore.SecurityList{s.securityLists[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListSecurityLists(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListSecurityLists(gomock.Any(), req).Return(ociCore.ListSecurityListsResponse{
		Items: []ociCore.SecurityList{s.securityLists[1]},
	}, nil)

	obtained, err := s.client.ListSecurityLists(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, s.securityLists)
}

func (s *networkClientSuite) TestListSecurityListsFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListSecurityListsRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	s.mockAPI.EXPECT().ListSecurityLists(gomock.Any(), req).Return(ociCore.ListSecurityListsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListSecurityLists(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) TestListSecurityListsFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	req := ociCore.ListSecurityListsRequest{
		CompartmentId: &compartmentID,
		VcnId:         s.vcns[0].Id,
	}
	resp := ociCore.ListSecurityListsResponse{
		Items:       []ociCore.SecurityList{s.securityLists[0]},
		OpcNextPage: new("test-pagination"),
	}
	s.mockAPI.EXPECT().ListSecurityLists(gomock.Any(), req).Return(resp, nil)
	req.Page = resp.OpcNextPage
	s.mockAPI.EXPECT().ListSecurityLists(gomock.Any(), req).Return(ociCore.ListSecurityListsResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListSecurityLists(ctx.TODO(), &compartmentID, s.vcns[0].Id)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *networkClientSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockAPI = testing.NewMockOCIVirtualNetworkingClient(ctrl)
	s.client = &networkClient{s.mockAPI}
	return ctrl
}

type storageClientSuite struct {
	client  *storageClient
	mockAPI *testing.MockOCIStorageClient
}

func (s *storageClientSuite) TestListVolumes(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	vol := newVolume(61440)
	s.mockAPI.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(ociCore.ListVolumesResponse{
		Items: []ociCore.Volume{vol},
	}, nil)

	obtained, err := s.client.ListVolumes(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, []ociCore.Volume{vol})
}

func (s *storageClientSuite) TestListVolumesPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	vol := newVolume(61440)
	s.mockAPI.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(ociCore.ListVolumesResponse{
		Items:       []ociCore.Volume{vol},
		OpcNextPage: new("test-pagination"),
	}, nil)

	vol2 := newVolume(87906)
	s.mockAPI.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(ociCore.ListVolumesResponse{
		Items: []ociCore.Volume{vol2},
	}, nil)

	obtained, err := s.client.ListVolumes(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, []ociCore.Volume{vol, vol2})
}

func (s *storageClientSuite) TestListVolumesFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockAPI.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(
		ociCore.ListVolumesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListVolumes(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *storageClientSuite) TestListVolumesFailPageN(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	vol := newVolume(61440)
	s.mockAPI.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(ociCore.ListVolumesResponse{
		Items:       []ociCore.Volume{vol},
		OpcNextPage: new("test-pagination"),
	}, nil)
	s.mockAPI.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(
		ociCore.ListVolumesResponse{}, errors.BadRequestf("test fail"))

	obtained, err := s.client.ListVolumes(ctx.TODO(), &compartmentID)
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
	c.Assert(obtained, tc.HasLen, 0)
}

func (s *storageClientSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockAPI = testing.NewMockOCIStorageClient(ctrl)
	s.client = &storageClient{s.mockAPI}
	return ctrl
}

func newVolume(size int64) ociCore.Volume {
	return ociCore.Volume{
		AvailabilityDomain: new("fakeZone1"),
		CompartmentId:      &compartmentID,
		Id:                 new("fakeVolumeId"),
		LifecycleState:     ociCore.VolumeLifecycleStateProvisioning,
		FreeformTags: map[string]string{
			tags.JujuModel: "fake-uuid",
		},
		SizeInGBs: &size,
	}
}
