// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"

	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	ociIdentity "github.com/oracle/oci-go-sdk/v65/identity"
)

// These interfaces represent the methods required by the OCI provider to
// interact with that cloud. They are not an exact match to the OCI API
// package due to some calls being paginated.  That is abstracted away from
// the provider in the clients provided by common, covering the ComputeClient,
// NetworkingClient, StorageClient and FirewallClient interfaces.

//go:generate go run go.uber.org/mock/mockgen -package testing -destination testing/mocks_identity.go -write_package_comment=false github.com/juju/juju/provider/oci IdentityClient
type IdentityClient interface {
	ListAvailabilityDomains(ctx context.Context, request ociIdentity.ListAvailabilityDomainsRequest) (ociIdentity.ListAvailabilityDomainsResponse, error)
}

//go:generate go run go.uber.org/mock/mockgen -package testing -destination testing/mocks_compute.go -write_package_comment=false github.com/juju/juju/provider/oci ComputeClient
type ComputeClient interface {
	ListVnicAttachments(ctx context.Context, compartmentID, instID *string) ([]ociCore.VnicAttachment, error)
	TerminateInstance(ctx context.Context, request ociCore.TerminateInstanceRequest) (ociCore.TerminateInstanceResponse, error)
	GetInstance(ctx context.Context, request ociCore.GetInstanceRequest) (ociCore.GetInstanceResponse, error)
	LaunchInstance(ctx context.Context, request ociCore.LaunchInstanceRequest) (ociCore.LaunchInstanceResponse, error)
	ListInstances(ctx context.Context, compartmentID *string) ([]ociCore.Instance, error)
	ListShapes(ctx context.Context, compartmentID, imageID *string) ([]ociCore.Shape, error)
	ListImages(ctx context.Context, compartmentID *string) ([]ociCore.Image, error)
	ListVolumeAttachments(ctx context.Context, compartmentID, instID *string) ([]ociCore.VolumeAttachment, error)
	GetVolumeAttachment(ctx context.Context, request ociCore.GetVolumeAttachmentRequest) (ociCore.GetVolumeAttachmentResponse, error)
	DetachVolume(ctx context.Context, request ociCore.DetachVolumeRequest) (ociCore.DetachVolumeResponse, error)
	AttachVolume(ctx context.Context, request ociCore.AttachVolumeRequest) (ociCore.AttachVolumeResponse, error)
}

//go:generate go run go.uber.org/mock/mockgen -package testing -destination testing/mocks_networking.go -write_package_comment=false github.com/juju/juju/provider/oci NetworkingClient
type NetworkingClient interface {
	CreateVcn(ctx context.Context, request ociCore.CreateVcnRequest) (ociCore.CreateVcnResponse, error)
	DeleteVcn(ctx context.Context, request ociCore.DeleteVcnRequest) (ociCore.DeleteVcnResponse, error)
	ListVcns(ctx context.Context, compartmentID *string) ([]ociCore.Vcn, error)
	GetVcn(ctx context.Context, request ociCore.GetVcnRequest) (ociCore.GetVcnResponse, error)

	CreateSubnet(ctx context.Context, request ociCore.CreateSubnetRequest) (ociCore.CreateSubnetResponse, error)
	ListSubnets(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.Subnet, error)
	DeleteSubnet(ctx context.Context, request ociCore.DeleteSubnetRequest) (ociCore.DeleteSubnetResponse, error)
	GetSubnet(ctx context.Context, request ociCore.GetSubnetRequest) (ociCore.GetSubnetResponse, error)

	CreateInternetGateway(ctx context.Context, request ociCore.CreateInternetGatewayRequest) (ociCore.CreateInternetGatewayResponse, error)
	GetInternetGateway(ctx context.Context, request ociCore.GetInternetGatewayRequest) (ociCore.GetInternetGatewayResponse, error)
	ListInternetGateways(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.InternetGateway, error)
	DeleteInternetGateway(ctx context.Context, request ociCore.DeleteInternetGatewayRequest) (ociCore.DeleteInternetGatewayResponse, error)

	CreateRouteTable(ctx context.Context, request ociCore.CreateRouteTableRequest) (ociCore.CreateRouteTableResponse, error)
	GetRouteTable(ctx context.Context, request ociCore.GetRouteTableRequest) (ociCore.GetRouteTableResponse, error)
	DeleteRouteTable(ctx context.Context, request ociCore.DeleteRouteTableRequest) (ociCore.DeleteRouteTableResponse, error)
	ListRouteTables(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.RouteTable, error)

	GetVnic(ctx context.Context, request ociCore.GetVnicRequest) (ociCore.GetVnicResponse, error)
}

//go:generate go run go.uber.org/mock/mockgen -package testing -destination testing/mocks_firewall.go -write_package_comment=false github.com/juju/juju/provider/oci FirewallClient
type FirewallClient interface {
	CreateSecurityList(ctx context.Context, request ociCore.CreateSecurityListRequest) (ociCore.CreateSecurityListResponse, error)
	ListSecurityLists(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.SecurityList, error)
	DeleteSecurityList(ctx context.Context, request ociCore.DeleteSecurityListRequest) (ociCore.DeleteSecurityListResponse, error)
	GetSecurityList(ctx context.Context, request ociCore.GetSecurityListRequest) (ociCore.GetSecurityListResponse, error)
}

//go:generate go run go.uber.org/mock/mockgen -package testing -destination testing/mocks_storage.go -write_package_comment=false github.com/juju/juju/provider/oci StorageClient
type StorageClient interface {
	CreateVolume(ctx context.Context, request ociCore.CreateVolumeRequest) (ociCore.CreateVolumeResponse, error)
	ListVolumes(ctx context.Context, compartmentID *string) ([]ociCore.Volume, error)
	GetVolume(ctx context.Context, request ociCore.GetVolumeRequest) (ociCore.GetVolumeResponse, error)
	DeleteVolume(ctx context.Context, request ociCore.DeleteVolumeRequest) (ociCore.DeleteVolumeResponse, error)
	UpdateVolume(ctx context.Context, request ociCore.UpdateVolumeRequest) (ociCore.UpdateVolumeResponse, error)
}
