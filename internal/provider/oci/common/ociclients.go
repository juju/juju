// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/oracle/oci-go-sdk/v65/common"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
)

// NewComputeClient returns a client which implements the
// OCIComputeClient interface.
func NewComputeClient(provider common.ConfigurationProvider) (*computeClient, error) {
	c, err := ociCore.NewComputeClientWithConfigurationProvider(provider)
	return &computeClient{c}, err
}

type computeClient struct {
	ociClient OCIComputeClient
}

// ListImages does the pagination work for the OCI Client's ListImages,
// returns a complete slice of Images.
func (c computeClient) ListImages(ctx context.Context, compartmentID *string) ([]ociCore.Image, error) {
	var images []ociCore.Image

	request := ociCore.ListImagesRequest{
		CompartmentId: compartmentID,
	}

	response, err := c.ociClient.ListImages(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing provider images")
	}
	images = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListImages(context.Background(), request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing provider images page %q", *request.Page)
		}
		images = append(images, response.Items...)
	}

	return images, nil
}

// ListShapes does the pagination work for the OCI Client's ListShapes,
// returns a complete slice of Shapes.
func (c computeClient) ListShapes(ctx context.Context, compartmentID, imageID *string) ([]ociCore.Shape, error) {
	var shapes []ociCore.Shape

	request := ociCore.ListShapesRequest{
		CompartmentId: compartmentID,
		ImageId:       imageID,
	}

	response, err := c.ociClient.ListShapes(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing image %s shapes", *imageID)
	}
	shapes = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListShapes(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing image %s shapes page %q", *imageID, *request.Page)
		}
		shapes = append(shapes, response.Items...)
	}

	return shapes, nil
}

// ListInstances does the pagination work for the OCI Client's ListInstances,
// returns a complete slice of Instances.
func (c computeClient) ListInstances(ctx context.Context, compartmentID *string) ([]ociCore.Instance, error) {
	var instances []ociCore.Instance

	request := ociCore.ListInstancesRequest{
		CompartmentId: compartmentID,
	}

	response, err := c.ociClient.ListInstances(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing instances")
	}
	instances = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListInstances(context.Background(), request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing instances page %q", *request.Page)
		}
		instances = append(instances, response.Items...)
	}

	return instances, nil
}

// ListVnicAttachments does the pagination work for the OCI Client's ListVnicAttachments,
// returns a complete slice of VnicAttachments for the given instance.
func (c computeClient) ListVnicAttachments(ctx context.Context, compartmentID, instID *string) ([]ociCore.VnicAttachment, error) {
	var attachments []ociCore.VnicAttachment

	request := ociCore.ListVnicAttachmentsRequest{
		CompartmentId: compartmentID,
		InstanceId:    instID,
	}

	response, err := c.ociClient.ListVnicAttachments(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing vnic attachments")
	}
	attachments = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListVnicAttachments(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing vnic attachments page %q", *request.Page)
		}
		attachments = append(attachments, response.Items...)
	}

	return attachments, nil
}

// ListVolumeAttachments does the pagination work for the OCI Client's ListVolumeAttachments,
// returns a complete slice of VolumeAttachments for the given instance.
func (c computeClient) ListVolumeAttachments(ctx context.Context, compartmentID, instID *string) ([]ociCore.VolumeAttachment, error) {
	var volumeAttachments []ociCore.VolumeAttachment

	request := ociCore.ListVolumeAttachmentsRequest{
		CompartmentId: compartmentID,
		InstanceId:    instID,
	}

	response, err := c.ociClient.ListVolumeAttachments(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing volume attachments for %s ", *instID)
	}
	volumeAttachments = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListVolumeAttachments(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing instance %s volumeAttachments page %q", *instID, *request.Page)
		}
		volumeAttachments = append(volumeAttachments, response.Items...)
	}

	return volumeAttachments, nil
}

func (c computeClient) TerminateInstance(ctx context.Context, request ociCore.TerminateInstanceRequest) (ociCore.TerminateInstanceResponse, error) {
	return c.ociClient.TerminateInstance(ctx, request)
}

func (c computeClient) GetInstance(ctx context.Context, request ociCore.GetInstanceRequest) (ociCore.GetInstanceResponse, error) {
	return c.ociClient.GetInstance(ctx, request)
}

func (c computeClient) LaunchInstance(ctx context.Context, request ociCore.LaunchInstanceRequest) (ociCore.LaunchInstanceResponse, error) {
	return c.ociClient.LaunchInstance(ctx, request)
}

func (c computeClient) GetVolumeAttachment(ctx context.Context, request ociCore.GetVolumeAttachmentRequest) (ociCore.GetVolumeAttachmentResponse, error) {
	return c.ociClient.GetVolumeAttachment(ctx, request)
}

func (c computeClient) DetachVolume(ctx context.Context, request ociCore.DetachVolumeRequest) (ociCore.DetachVolumeResponse, error) {
	return c.ociClient.DetachVolume(ctx, request)
}

func (c computeClient) AttachVolume(ctx context.Context, request ociCore.AttachVolumeRequest) (ociCore.AttachVolumeResponse, error) {
	return c.ociClient.AttachVolume(ctx, request)
}

// NewNetworkClient returns a client which implements the
// OCINetworkingClient and OCIFirewallClient interfaces.
func NewNetworkClient(provider common.ConfigurationProvider) (*networkClient, error) {
	c, err := ociCore.NewVirtualNetworkClientWithConfigurationProvider(provider)
	return &networkClient{c}, err
}

type networkClient struct {
	ociClient OCIVirtualNetworkingClient
}

// ListVcns does the pagination work for the OCI Client's ListVcns,
// returns a complete slice of Vcns.
func (c networkClient) ListVcns(ctx context.Context, compartmentID *string) ([]ociCore.Vcn, error) {
	var vncs []ociCore.Vcn

	request := ociCore.ListVcnsRequest{
		CompartmentId: compartmentID,
	}

	response, err := c.ociClient.ListVcns(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing vcns")
	}
	vncs = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListVcns(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing vcns page %q", *request.Page)
		}
		vncs = append(vncs, response.Items...)
	}

	return vncs, nil
}

// ListSubnets does the pagination work for the OCI Client's ListSubnets,
// returns a complete slice of Subnets.
func (c networkClient) ListSubnets(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.Subnet, error) {
	var subnets []ociCore.Subnet

	request := ociCore.ListSubnetsRequest{
		CompartmentId: compartmentID,
		VcnId:         vcnID,
	}

	response, err := c.ociClient.ListSubnets(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing subnets for %s ", *vcnID)
	}
	subnets = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListSubnets(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing subnets for %s page %q", *vcnID, *request.Page)
		}
		subnets = append(subnets, response.Items...)
	}

	return subnets, nil
}

// ListInternetGateways does the pagination work for the OCI Client's ListInternetGateways,
// returns a complete slice of InternetGateways.
func (c networkClient) ListInternetGateways(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.InternetGateway, error) {
	var internetGateways []ociCore.InternetGateway

	request := ociCore.ListInternetGatewaysRequest{
		CompartmentId: compartmentID,
		VcnId:         vcnID,
	}

	response, err := c.ociClient.ListInternetGateways(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing internet gatesways for %s ", *vcnID)
	}
	internetGateways = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListInternetGateways(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing internet gatesways for %s page %q", *vcnID, *request.Page)
		}
		internetGateways = append(internetGateways, response.Items...)
	}

	return internetGateways, nil
}

// ListRouteTables does the pagination work for the OCI Client's ListRouteTables,
// returns a complete slice of RouteTables.
func (c networkClient) ListRouteTables(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.RouteTable, error) {
	var routeTables []ociCore.RouteTable

	request := ociCore.ListRouteTablesRequest{
		CompartmentId: compartmentID,
		VcnId:         vcnID,
	}

	response, err := c.ociClient.ListRouteTables(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing route tables for %s ", *vcnID)
	}
	routeTables = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListRouteTables(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing route tables for %s page %q", *vcnID, *request.Page)
		}
		routeTables = append(routeTables, response.Items...)
	}

	return routeTables, nil
}

// ListSecurityLists does the pagination work for the OCI Client's ListSecurityLists,
// returns a complete slice of SecurityLists.
func (c networkClient) ListSecurityLists(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.SecurityList, error) {
	var securityLists []ociCore.SecurityList

	request := ociCore.ListSecurityListsRequest{
		CompartmentId: compartmentID,
		VcnId:         vcnID,
	}

	response, err := c.ociClient.ListSecurityLists(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing security lists for %s ", *vcnID)
	}
	securityLists = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListSecurityLists(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing security lists  for %s page %q", *vcnID, *request.Page)
		}
		securityLists = append(securityLists, response.Items...)
	}

	return securityLists, nil
}

func (c networkClient) CreateVcn(ctx context.Context, request ociCore.CreateVcnRequest) (ociCore.CreateVcnResponse, error) {
	return c.ociClient.CreateVcn(ctx, request)
}

func (c networkClient) DeleteVcn(ctx context.Context, request ociCore.DeleteVcnRequest) (ociCore.DeleteVcnResponse, error) {
	return c.ociClient.DeleteVcn(ctx, request)
}

func (c networkClient) GetVcn(ctx context.Context, request ociCore.GetVcnRequest) (ociCore.GetVcnResponse, error) {
	return c.ociClient.GetVcn(ctx, request)
}

func (c networkClient) CreateSubnet(ctx context.Context, request ociCore.CreateSubnetRequest) (ociCore.CreateSubnetResponse, error) {
	return c.ociClient.CreateSubnet(ctx, request)
}

func (c networkClient) DeleteSubnet(ctx context.Context, request ociCore.DeleteSubnetRequest) (ociCore.DeleteSubnetResponse, error) {
	return c.ociClient.DeleteSubnet(ctx, request)
}

func (c networkClient) GetSubnet(ctx context.Context, request ociCore.GetSubnetRequest) (ociCore.GetSubnetResponse, error) {
	return c.ociClient.GetSubnet(ctx, request)
}

func (c networkClient) CreateInternetGateway(ctx context.Context, request ociCore.CreateInternetGatewayRequest) (ociCore.CreateInternetGatewayResponse, error) {
	return c.ociClient.CreateInternetGateway(ctx, request)
}

func (c networkClient) GetInternetGateway(ctx context.Context, request ociCore.GetInternetGatewayRequest) (ociCore.GetInternetGatewayResponse, error) {
	return c.ociClient.GetInternetGateway(ctx, request)
}

func (c networkClient) DeleteInternetGateway(ctx context.Context, request ociCore.DeleteInternetGatewayRequest) (ociCore.DeleteInternetGatewayResponse, error) {
	return c.ociClient.DeleteInternetGateway(ctx, request)
}

func (c networkClient) CreateRouteTable(ctx context.Context, request ociCore.CreateRouteTableRequest) (ociCore.CreateRouteTableResponse, error) {
	return c.ociClient.CreateRouteTable(ctx, request)
}

func (c networkClient) GetRouteTable(ctx context.Context, request ociCore.GetRouteTableRequest) (ociCore.GetRouteTableResponse, error) {
	return c.ociClient.GetRouteTable(ctx, request)
}

func (c networkClient) DeleteRouteTable(ctx context.Context, request ociCore.DeleteRouteTableRequest) (ociCore.DeleteRouteTableResponse, error) {
	return c.ociClient.DeleteRouteTable(ctx, request)
}

func (c networkClient) GetVnic(ctx context.Context, request ociCore.GetVnicRequest) (ociCore.GetVnicResponse, error) {
	return c.ociClient.GetVnic(ctx, request)
}

func (c networkClient) CreateSecurityList(ctx context.Context, request ociCore.CreateSecurityListRequest) (ociCore.CreateSecurityListResponse, error) {
	return c.ociClient.CreateSecurityList(ctx, request)
}

func (c networkClient) DeleteSecurityList(ctx context.Context, request ociCore.DeleteSecurityListRequest) (ociCore.DeleteSecurityListResponse, error) {
	return c.ociClient.DeleteSecurityList(ctx, request)
}

func (c networkClient) GetSecurityList(ctx context.Context, request ociCore.GetSecurityListRequest) (ociCore.GetSecurityListResponse, error) {
	return c.ociClient.GetSecurityList(ctx, request)
}

// NewStorageClient returns a client which implements the
// OCIStorageClient interface.
func NewStorageClient(provider common.ConfigurationProvider) (*storageClient, error) {
	c, err := ociCore.NewBlockstorageClientWithConfigurationProvider(provider)
	return &storageClient{c}, err
}

type storageClient struct {
	ociClient OCIStorageClient
}

// ListVolumes does the pagination work for the OCI Client's ListVolumes,
// returns a complete slice of Volumes.
func (c storageClient) ListVolumes(ctx context.Context, compartmentID *string) ([]ociCore.Volume, error) {
	var volumes []ociCore.Volume

	request := ociCore.ListVolumesRequest{
		CompartmentId: compartmentID,
	}

	response, err := c.ociClient.ListVolumes(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing volumes")
	}
	volumes = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ociClient.ListVolumes(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing volumes page %q", *request.Page)
		}
		volumes = append(volumes, response.Items...)
	}

	return volumes, nil
}

func (c storageClient) CreateVolume(ctx context.Context, request ociCore.CreateVolumeRequest) (ociCore.CreateVolumeResponse, error) {
	return c.ociClient.CreateVolume(ctx, request)
}

func (c storageClient) GetVolume(ctx context.Context, request ociCore.GetVolumeRequest) (ociCore.GetVolumeResponse, error) {
	return c.ociClient.GetVolume(ctx, request)
}

func (c storageClient) DeleteVolume(ctx context.Context, request ociCore.DeleteVolumeRequest) (ociCore.DeleteVolumeResponse, error) {
	return c.ociClient.DeleteVolume(ctx, request)
}

func (c storageClient) UpdateVolume(ctx context.Context, request ociCore.UpdateVolumeRequest) (ociCore.UpdateVolumeResponse, error) {
	return c.ociClient.UpdateVolume(ctx, request)
}
