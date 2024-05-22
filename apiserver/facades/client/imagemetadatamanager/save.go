// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/cloudimagemetadata"
)

// ImageMetadataSaver is an interface for manipulating images metadata.
type ImageMetadataSaver interface {
	// SaveMetadata persists collection of given images metadata.
	SaveMetadata([]cloudimagemetadata.Metadata) error
}

// Save stores given cloud image metadata using given persistence interface.
func Save(
	ctx context.Context,
	service ModelConfigService,
	saver ImageMetadataSaver,
	metadata params.MetadataSaveParams,
) ([]params.ErrorResult, error) {
	all := make([]params.ErrorResult, len(metadata.Metadata))
	if len(metadata.Metadata) == 0 {
		return nil, nil
	}
	modelCfg, err := service.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "getting model config")
	}
	for i, one := range metadata.Metadata {
		md := ParseMetadataListFromParams(one, modelCfg)
		err := saver.SaveMetadata(md)
		all[i] = params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}
	return all, nil
}

// ParseMetadataListFromParams translates params.CloudImageMetadataList
// into a collection of cloudimagemetadata.Metadata.
func ParseMetadataListFromParams(p params.CloudImageMetadataList, cfg *config.Config) []cloudimagemetadata.Metadata {
	results := make([]cloudimagemetadata.Metadata, len(p.Metadata))
	for i, metadata := range p.Metadata {
		results[i] = cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          metadata.Stream,
				Region:          metadata.Region,
				Version:         metadata.Version,
				Arch:            metadata.Arch,
				VirtType:        metadata.VirtType,
				RootStorageType: metadata.RootStorageType,
				RootStorageSize: metadata.RootStorageSize,
				Source:          metadata.Source,
			},
			Priority: metadata.Priority,
			ImageId:  metadata.ImageId,
		}
		if metadata.Source == "custom" && metadata.Priority == 0 {
			results[i].Priority = simplestreams.CUSTOM_CLOUD_DATA
		}
		// TODO (anastasiamac 2016-08-24) This is a band-aid solution.
		// Once correct value is read from simplestreams, this needs to go.
		// Bug# 1616295
		if results[i].Stream == "" {
			results[i].Stream = cfg.ImageStream()
		}
	}
	return results
}
