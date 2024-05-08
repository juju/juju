// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	ociCore "github.com/oracle/oci-go-sdk/v65/core"
)

// These interfaces represent the methods required for the OCI Provider to interact with
// that cloud for compute, storage and virtual networking.  This is due to abstracting away
// pagination required by most List methods.

//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination ../testing/mocks_clientcompute.go -write_package_comment=false github.com/juju/juju/internal/provider/oci/common OCIComputeClient
type OCIComputeClient interface {
	ListVnicAttachments(ctx context.Context, request ociCore.ListVnicAttachmentsRequest) (ociCore.ListVnicAttachmentsResponse, error)
	TerminateInstance(ctx context.Context, request ociCore.TerminateInstanceRequest) (ociCore.TerminateInstanceResponse, error)
	GetInstance(ctx context.Context, request ociCore.GetInstanceRequest) (ociCore.GetInstanceResponse, error)
	LaunchInstance(ctx context.Context, request ociCore.LaunchInstanceRequest) (ociCore.LaunchInstanceResponse, error)
	ListInstances(ctx context.Context, request ociCore.ListInstancesRequest) (ociCore.ListInstancesResponse, error)
	ListShapes(ctx context.Context, request ociCore.ListShapesRequest) (ociCore.ListShapesResponse, error)
	ListImages(ctx context.Context, request ociCore.ListImagesRequest) (ociCore.ListImagesResponse, error)
	ListVolumeAttachments(ctx context.Context, request ociCore.ListVolumeAttachmentsRequest) (ociCore.ListVolumeAttachmentsResponse, error)
	GetVolumeAttachment(ctx context.Context, request ociCore.GetVolumeAttachmentRequest) (ociCore.GetVolumeAttachmentResponse, error)
	DetachVolume(ctx context.Context, request ociCore.DetachVolumeRequest) (ociCore.DetachVolumeResponse, error)
	AttachVolume(ctx context.Context, request ociCore.AttachVolumeRequest) (ociCore.AttachVolumeResponse, error)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination ../testing/mocks_clientnetworking.go -write_package_comment=false github.com/juju/juju/internal/provider/oci/common OCIVirtualNetworkingClient
type OCIVirtualNetworkingClient interface {
	CreateVcn(ctx context.Context, request ociCore.CreateVcnRequest) (ociCore.CreateVcnResponse, error)
	DeleteVcn(ctx context.Context, request ociCore.DeleteVcnRequest) (ociCore.DeleteVcnResponse, error)
	ListVcns(ctx context.Context, request ociCore.ListVcnsRequest) (ociCore.ListVcnsResponse, error)
	GetVcn(ctx context.Context, request ociCore.GetVcnRequest) (ociCore.GetVcnResponse, error)

	CreateSubnet(ctx context.Context, request ociCore.CreateSubnetRequest) (ociCore.CreateSubnetResponse, error)
	ListSubnets(ctx context.Context, request ociCore.ListSubnetsRequest) (ociCore.ListSubnetsResponse, error)
	DeleteSubnet(ctx context.Context, request ociCore.DeleteSubnetRequest) (ociCore.DeleteSubnetResponse, error)
	GetSubnet(ctx context.Context, request ociCore.GetSubnetRequest) (ociCore.GetSubnetResponse, error)

	CreateInternetGateway(ctx context.Context, request ociCore.CreateInternetGatewayRequest) (ociCore.CreateInternetGatewayResponse, error)
	GetInternetGateway(ctx context.Context, request ociCore.GetInternetGatewayRequest) (ociCore.GetInternetGatewayResponse, error)
	ListInternetGateways(ctx context.Context, request ociCore.ListInternetGatewaysRequest) (response ociCore.ListInternetGatewaysResponse, err error)
	DeleteInternetGateway(ctx context.Context, request ociCore.DeleteInternetGatewayRequest) (ociCore.DeleteInternetGatewayResponse, error)

	CreateRouteTable(ctx context.Context, request ociCore.CreateRouteTableRequest) (ociCore.CreateRouteTableResponse, error)
	GetRouteTable(ctx context.Context, request ociCore.GetRouteTableRequest) (ociCore.GetRouteTableResponse, error)
	DeleteRouteTable(ctx context.Context, request ociCore.DeleteRouteTableRequest) (ociCore.DeleteRouteTableResponse, error)
	ListRouteTables(ctx context.Context, request ociCore.ListRouteTablesRequest) (response ociCore.ListRouteTablesResponse, err error)

	GetVnic(ctx context.Context, request ociCore.GetVnicRequest) (ociCore.GetVnicResponse, error)

	CreateSecurityList(ctx context.Context, request ociCore.CreateSecurityListRequest) (ociCore.CreateSecurityListResponse, error)
	ListSecurityLists(ctx context.Context, request ociCore.ListSecurityListsRequest) (ociCore.ListSecurityListsResponse, error)
	DeleteSecurityList(ctx context.Context, request ociCore.DeleteSecurityListRequest) (ociCore.DeleteSecurityListResponse, error)
	GetSecurityList(ctx context.Context, request ociCore.GetSecurityListRequest) (ociCore.GetSecurityListResponse, error)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination ../testing/mocks_clientstorage.go -write_package_comment=false github.com/juju/juju/internal/provider/oci/common OCIStorageClient
type OCIStorageClient interface {
	CreateVolume(ctx context.Context, request ociCore.CreateVolumeRequest) (ociCore.CreateVolumeResponse, error)
	ListVolumes(ctx context.Context, request ociCore.ListVolumesRequest) (response ociCore.ListVolumesResponse, err error)
	GetVolume(ctx context.Context, request ociCore.GetVolumeRequest) (ociCore.GetVolumeResponse, error)
	DeleteVolume(ctx context.Context, request ociCore.DeleteVolumeRequest) (ociCore.DeleteVolumeResponse, error)
	UpdateVolume(ctx context.Context, request ociCore.UpdateVolumeRequest) (ociCore.UpdateVolumeResponse, error)
}
