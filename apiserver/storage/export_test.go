// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	IsValidPoolListFilter      = (*API).isValidPoolListFilter
	ValidateNames              = (*API).isValidNameCriteria
	ValidateProviders          = (*API).isValidProviderCriteria
	CreateVolumeItem           = (*API).createVolumeItem
	GetVolumeItems             = (*API).getVolumeItems
	FilterVolumes              = (*API).filterVolumes
	VolumeAttachments          = (*API).volumeAttachments
	ListVolumeAttachments      = (*API).listVolumeAttachments
	ConvertStateVolumeToParams = (*API).convertStateVolumeToParams

	CreateAPI                             = createAPI
	GroupAttachmentsByVolume              = groupAttachmentsByVolume
	ConvertStateVolumeAttachmentToParams  = convertStateVolumeAttachmentToParams
	ConvertStateVolumeAttachmentsToParams = convertStateVolumeAttachmentsToParams
)
