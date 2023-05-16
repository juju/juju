// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"sort"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/imagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/cloudimagemetadata"
)

// API is the concrete implementation of the api end point
// for loud image metadata manipulations.
type API struct {
	metadata   metadataAccess
	newEnviron func() (environs.Environ, error)
}

// createAPI returns a new image metadata API facade.
func createAPI(
	st metadataAccess,
	newEnviron func() (environs.Environ, error),
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	err := authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag())
	if err != nil {
		return nil, err
	}

	return &API{
		metadata:   st,
		newEnviron: newEnviron,
	}, nil
}

// List returns all found cloud image metadata that satisfy
// given filter.
// Returned list contains metadata ordered by priority.
func (api *API) List(filter params.ImageMetadataFilter) (params.ListCloudImageMetadataResult, error) {
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
func (api *API) Save(metadata params.MetadataSaveParams) (params.ErrorResults, error) {
	model, err := api.metadata.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for _, mList := range metadata.Metadata {
		for i, m := range mList.Metadata {
			if m.Region == "" {
				m.Region = model.CloudRegion()
				mList.Metadata[i] = m
			}
		}
	}
	all, err := imagecommon.Save(api.metadata, metadata)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return params.ErrorResults{Results: all}, nil
}

// Delete deletes cloud image metadata for given image ids.
// It supports bulk calls.
func (api *API) Delete(images params.MetadataImageIds) (params.ErrorResults, error) {
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
