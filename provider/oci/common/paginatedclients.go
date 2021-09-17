// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/oracle/oci-go-sdk/v47/common"
	ociCore "github.com/oracle/oci-go-sdk/v47/core"
)

// NewComputeClient returns a client which implements the
// OCIComputeClient interface.
func NewComputeClient(provider common.ConfigurationProvider) (*computeClient, error) {
	c, err := ociCore.NewComputeClientWithConfigurationProvider(provider)
	return &computeClient{c}, err
}

type computeClient struct {
	OCIComputeClient
}

// PaginatedListImages does the pagination work for ListImages,
// returns a complete slice of Images.
func (c computeClient) PaginatedListImages(ctx context.Context, compartmentID *string) ([]ociCore.Image, error) {
	var images []ociCore.Image

	request := ociCore.ListImagesRequest{
		CompartmentId: compartmentID,
	}

	response, err := c.ListImages(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing provider images")
	}
	images = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListImages(context.Background(), request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing provider images page %q", *request.Page)
		}
		images = append(images, response.Items...)
	}

	return images, nil
}

// PaginatedListShapes does the pagination work for ListShapes,
// returns a complete slice of Shapes.
func (c computeClient) PaginatedListShapes(ctx context.Context, compartmentID, imageID *string) ([]ociCore.Shape, error) {
	var shapes []ociCore.Shape

	request := ociCore.ListShapesRequest{
		CompartmentId: compartmentID,
		ImageId:       imageID,
	}

	response, err := c.ListShapes(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing image %s shapes", *imageID)
	}
	shapes = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListShapes(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing image %s shapes page %q", *imageID, *request.Page)
		}
		shapes = append(shapes, response.Items...)
	}

	return shapes, nil
}

// PaginatedListInstances does the pagination work for ListInstances,
// returns a complete slice of Instances.
func (c computeClient) PaginatedListInstances(ctx context.Context, compartmentID *string) ([]ociCore.Instance, error) {
	var instances []ociCore.Instance

	request := ociCore.ListInstancesRequest{
		CompartmentId: compartmentID,
	}

	response, err := c.ListInstances(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing instances")
	}
	instances = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListInstances(context.Background(), request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing instances page %q", *request.Page)
		}
		instances = append(instances, response.Items...)
	}

	return instances, nil
}

// PaginatedListVnicAttachments does the pagination work for ListVnicAttachments,
// returns a complete slice of VnicAttachments for the given instance.
func (c computeClient) PaginatedListVnicAttachments(ctx context.Context, compartmentID, instID *string) ([]ociCore.VnicAttachment, error) {
	var attachments []ociCore.VnicAttachment

	request := ociCore.ListVnicAttachmentsRequest{
		CompartmentId: compartmentID,
		InstanceId:    instID,
	}

	response, err := c.ListVnicAttachments(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing vnic attachments")
	}
	attachments = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListVnicAttachments(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing vnic attachments page %q", *request.Page)
		}
		attachments = append(attachments, response.Items...)
	}

	return attachments, nil
}

// PaginatedListVolumeAttachments does the pagination work for ListVolumeAttachments,
// returns a complete slice of VolumeAttachments for the given instance.
func (c computeClient) PaginatedListVolumeAttachments(ctx context.Context, compartmentID, instID *string) ([]ociCore.VolumeAttachment, error) {
	var volumeAttachments []ociCore.VolumeAttachment

	request := ociCore.ListVolumeAttachmentsRequest{
		CompartmentId: compartmentID,
		InstanceId:    instID,
	}

	response, err := c.ListVolumeAttachments(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing volume attachments for %s ", *instID)
	}
	volumeAttachments = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListVolumeAttachments(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing instance %s volumeAttachments page %q", *instID, *request.Page)
		}
		volumeAttachments = append(volumeAttachments, response.Items...)
	}

	return volumeAttachments, nil
}

// NewNetworkClient returns a client which implements the
// OCINetworkingClient and OCIFirewallClient interfaces.
func NewNetworkClient(provider common.ConfigurationProvider) (*networkClient, error) {
	c, err := ociCore.NewVirtualNetworkClientWithConfigurationProvider(provider)
	return &networkClient{c}, err
}

type networkClient struct {
	OCIVirtualNetworkingClient
}

// PaginatedListVcns does the pagination work for ListVcns,
// returns a complete slice of Vcns.
func (c networkClient) PaginatedListVcns(ctx context.Context, compartmentID *string) ([]ociCore.Vcn, error) {
	var vncs []ociCore.Vcn

	request := ociCore.ListVcnsRequest{
		CompartmentId: compartmentID,
	}

	response, err := c.ListVcns(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing vcns")
	}
	vncs = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListVcns(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing vcns page %q", *request.Page)
		}
		vncs = append(vncs, response.Items...)
	}

	return vncs, nil
}

// PaginatedListSubnets does the pagination work for ListSubnets,
// returns a complete slice of Subnets.
func (c networkClient) PaginatedListSubnets(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.Subnet, error) {
	var subnets []ociCore.Subnet

	request := ociCore.ListSubnetsRequest{
		CompartmentId: compartmentID,
		VcnId:         vcnID,
	}

	response, err := c.ListSubnets(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing subnets for %s ", *vcnID)
	}
	subnets = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListSubnets(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing subnets for %s page %q", *vcnID, *request.Page)
		}
		subnets = append(subnets, response.Items...)
	}

	return subnets, nil
}

// PaginatedListInternetGateways does the pagination work for ListInternetGateways,
// returns a complete slice of InternetGateways.
func (c networkClient) PaginatedListInternetGateways(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.InternetGateway, error) {
	var internetGateways []ociCore.InternetGateway

	request := ociCore.ListInternetGatewaysRequest{
		CompartmentId: compartmentID,
		VcnId:         vcnID,
	}

	response, err := c.ListInternetGateways(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing internet gatesways for %s ", *vcnID)
	}
	internetGateways = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListInternetGateways(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing internet gatesways for %s page %q", *vcnID, *request.Page)
		}
		internetGateways = append(internetGateways, response.Items...)
	}

	return internetGateways, nil
}

// PaginatedListRouteTables does the pagination work for ListRouteTables,
// returns a complete slice of RouteTables.
func (c networkClient) PaginatedListRouteTables(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.RouteTable, error) {
	var routeTables []ociCore.RouteTable

	request := ociCore.ListRouteTablesRequest{
		CompartmentId: compartmentID,
		VcnId:         vcnID,
	}

	response, err := c.ListRouteTables(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing route tables for %s ", *vcnID)
	}
	routeTables = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListRouteTables(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing route tables for %s page %q", *vcnID, *request.Page)
		}
		routeTables = append(routeTables, response.Items...)
	}

	return routeTables, nil
}

// PaginatedListSecurityLists does the pagination work for ListSecurityLists,
// returns a complete slice of SecurityLists.
func (c networkClient) PaginatedListSecurityLists(ctx context.Context, compartmentID, vcnID *string) ([]ociCore.SecurityList, error) {
	var securityLists []ociCore.SecurityList

	request := ociCore.ListSecurityListsRequest{
		CompartmentId: compartmentID,
		VcnId:         vcnID,
	}

	response, err := c.ListSecurityLists(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing security lists for %s ", *vcnID)
	}
	securityLists = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListSecurityLists(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing security lists  for %s page %q", *vcnID, *request.Page)
		}
		securityLists = append(securityLists, response.Items...)
	}

	return securityLists, nil
}

// NewStorageClient returns a client which implements the
// OCIStorageClient interface.
func NewStorageClient(provider common.ConfigurationProvider) (*storageClient, error) {
	c, err := ociCore.NewBlockstorageClientWithConfigurationProvider(provider)
	return &storageClient{c}, err
}

type storageClient struct {
	OCIStorageClient
}

// PaginatedListVolumes does the pagination work for ListVolumes,
// returns a complete slice of Volumes.
func (c storageClient) PaginatedListVolumes(ctx context.Context, compartmentID *string) ([]ociCore.Volume, error) {
	var volumes []ociCore.Volume

	request := ociCore.ListVolumesRequest{
		CompartmentId: compartmentID,
	}

	response, err := c.ListVolumes(ctx, request)
	if err != nil {
		return nil, errors.Annotatef(err, "listing volumes")
	}
	volumes = response.Items

	for response.OpcNextPage != nil {
		request.Page = response.OpcNextPage
		response, err = c.ListVolumes(ctx, request)
		if err != nil {
			return nil, errors.Annotatef(err, "listing volumes page %q", *request.Page)
		}
		volumes = append(volumes, response.Items...)
	}

	return volumes, nil
}
