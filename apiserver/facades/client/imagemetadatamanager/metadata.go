// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"context"
	"sort"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/cloudimagemetadata"
)

// API is the concrete implementation of the API endpoint for cloud image
// metadata manipulations.
type API struct {
	metadata   metadataAccess
	newEnviron func() (environs.Environ, error)

	modelConfigService ModelConfigService
	modelInfoService   ModelInfoService
}

// newAPI is responsible for constructing a new [API]
func newAPI(
	st metadataAccess,
	modelConfigService ModelConfigService,
	modelInfoService ModelInfoService,
	newEnviron func() (environs.Environ, error),
) *API {
	return &API{
		metadata:           st,
		newEnviron:         newEnviron,
		modelConfigService: modelConfigService,
		modelInfoService:   modelInfoService,
	}
}

// List returns all found cloud image metadata that satisfy
// given filter.
// Returned list contains metadata ordered by priority.
func (api *API) List(ctx context.Context, filter params.ImageMetadataFilter) (params.ListCloudImageMetadataResult, error) {
	found, err := api.metadata.FindMetadata(cloudimagemetadata.MetadataFilter{
		Region:          filter.Region,
		Versions:        filter.Versions,
		Arches:          filter.Arches,
		Stream:          filter.Stream,
		VirtType:        filter.VirtType,
		RootStorageType: filter.RootStorageType,
	})
	if err != nil {
		return params.ListCloudImageMetadataResult{}, apiservererrors.ServerError(err)
	}

	var all []params.CloudImageMetadata
	addAll := func(ms []cloudimagemetadata.Metadata) {
		for _, m := range ms {
			all = append(all, parseMetadataToParams(m))
		}
	}

	for _, ms := range found {
		addAll(ms)
	}
	sort.Sort(metadataList(all))

	return params.ListCloudImageMetadataResult{Result: all}, nil
}

// Save stores given cloud image metadata.
// It supports bulk calls.
func (api *API) Save(ctx context.Context, metadata params.MetadataSaveParams) (params.ErrorResults, error) {
	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	for _, mList := range metadata.Metadata {
		for i, m := range mList.Metadata {
			if m.Region == "" {
				m.Region = modelInfo.CloudRegion
				mList.Metadata[i] = m
			}
		}
	}

	all, err := Save(ctx, api.modelConfigService, api.metadata, metadata)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return params.ErrorResults{Results: all}, nil
}

// Delete deletes cloud image metadata for given image ids.
// It supports bulk calls.
func (api *API) Delete(ctx context.Context, images params.MetadataImageIds) (params.ErrorResults, error) {
	all := make([]params.ErrorResult, len(images.Ids))
	for i, imageId := range images.Ids {
		err := api.metadata.DeleteMetadata(imageId)
		all[i] = params.ErrorResult{apiservererrors.ServerError(err)}
	}
	return params.ErrorResults{Results: all}, nil
}

func parseMetadataToParams(p cloudimagemetadata.Metadata) params.CloudImageMetadata {
	result := params.CloudImageMetadata{
		ImageId:         p.ImageId,
		Stream:          p.Stream,
		Region:          p.Region,
		Version:         p.Version,
		Arch:            p.Arch,
		VirtType:        p.VirtType,
		RootStorageType: p.RootStorageType,
		RootStorageSize: p.RootStorageSize,
		Source:          p.Source,
		Priority:        p.Priority,
	}
	return result
}

// metadataList is a convenience type enabling to sort
// a collection of Metadata in order of priority.
type metadataList []params.CloudImageMetadata

// Len implements sort.Interface
func (m metadataList) Len() int {
	return len(m)
}

// Less implements sort.Interface and sorts image metadata by priority.
func (m metadataList) Less(i, j int) bool {
	return m[i].Priority < m[j].Priority
}

// Swap implements sort.Interface
func (m metadataList) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}
