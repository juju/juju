// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coremachine "github.com/juju/juju/core/machine"
	provisioning "github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ProvisioningInfo returns the provisioning information for each given machine entity.
// It supports all positive space constraints.
func (api *ProvisionerAPI) ProvisioningInfo(ctx context.Context, args params.Entities) (params.ProvisioningInfoResults, error) {
	result := params.ProvisioningInfoResults{
		Results: make([]params.ProvisioningInfoResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, errors.Capture(err)
	}

	controllerConfig, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Errorf("getting controller config: %w", err)
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil || !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := coremachine.Name(tag.Id())

		info, err := api.provisioningService.GetProvisioningInfo(ctx, machineName, api.isControllerModel, controllerConfig)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		pInfo := provisioningInfoToParams(info)
		result.Results[i].Result = &pInfo
	}
	return result, nil
}

// provisioningInfoToParams converts the domain provisioning info type to the
// API params type.
func provisioningInfoToParams(info provisioning.ProvisioningInfo) params.ProvisioningInfo {
	result := params.ProvisioningInfo{
		Base: params.Base{
			Name:    info.Base.OS,
			Channel: info.Base.Channel.String(),
		},
		Constraints:       info.Constraints,
		Placement:         unptr(info.PlacementDirective),
		Jobs:              info.Jobs,
		Tags:              info.Tags,
		EndpointBindings:  info.EndpointBindings,
		CloudInitUserData: info.CloudInitUserData,
		ControllerConfig:  info.ControllerConfig,
	}

	// Convert volumes.
	if len(info.Volumes) > 0 {
		result.Volumes = make([]params.VolumeParams, len(info.Volumes))
		for i, v := range info.Volumes {
			result.Volumes[i] = volumeParamsToParams(v)
		}
	}

	// Convert volume attachments.
	if len(info.VolumeAttachments) > 0 {
		result.VolumeAttachments = make([]params.VolumeAttachmentParams, len(info.VolumeAttachments))
		for i, va := range info.VolumeAttachments {
			result.VolumeAttachments[i] = volumeAttachmentParamsToParams(va)
		}
	}

	// Convert root disk.
	if info.RootDisk != nil {
		rd := volumeParamsToParams(*info.RootDisk)
		result.RootDisk = &rd
	}

	// Convert image metadata.
	if len(info.ImageMetadata) > 0 {
		result.ImageMetadata = make([]params.CloudImageMetadata, len(info.ImageMetadata))
		for i, m := range info.ImageMetadata {
			result.ImageMetadata[i] = params.CloudImageMetadata{
				ImageId:         m.ImageID,
				Stream:          m.Stream,
				Region:          m.Region,
				Version:         m.Version,
				Arch:            m.Arch,
				VirtType:        m.VirtType,
				RootStorageType: m.RootStorageType,
				RootStorageSize: m.RootStorageSize,
				Source:          m.Source,
				Priority:        m.Priority,
			}
		}
	}

	// Convert network topology.
	if info.SpaceSubnets != nil || info.SubnetAZs != nil {
		result.ProvisioningNetworkTopology = params.ProvisioningNetworkTopology{
			SubnetAZs:    info.SubnetAZs,
			SpaceSubnets: info.SpaceSubnets,
		}
	}

	return result
}

// volumeParamsToParams converts domain volume params to API params.
func volumeParamsToParams(v provisioning.VolumeParams) params.VolumeParams {
	p := params.VolumeParams{
		SizeMiB:    v.SizeMiB,
		Provider:   v.Provider,
		Attributes: v.Attributes,
		Tags:       v.Tags,
	}
	if v.VolumeID != "" {
		p.VolumeTag = names.NewVolumeTag(v.VolumeID).String()
	}
	if v.Attachment != nil {
		att := volumeAttachmentParamsToParams(*v.Attachment)
		p.Attachment = &att
	}
	return p
}

// volumeAttachmentParamsToParams converts domain volume attachment params
// to API params.
func volumeAttachmentParamsToParams(va provisioning.VolumeAttachmentParams) params.VolumeAttachmentParams {
	p := params.VolumeAttachmentParams{
		Provider:   va.Provider,
		ReadOnly:   va.ReadOnly,
		ProviderId: va.ProviderID,
	}
	if va.VolumeID != "" {
		p.VolumeTag = names.NewVolumeTag(va.VolumeID).String()
	}
	if va.MachineID != "" {
		p.MachineTag = names.NewMachineTag(va.MachineID).String()
	}
	return p
}

func unptr[T any](ptr *T) T {
	var zero T
	if ptr == nil {
		return zero
	}
	return *ptr
}
