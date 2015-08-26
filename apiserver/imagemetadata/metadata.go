// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"fmt"
	"strings"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
)

func init() {
	common.RegisterStandardFacade("ImageMetadata", 1, NewAPI)
}

// API is the concrete implementation of the api end point
// for loud image metadata manipulations.
type API struct {
	metadata   metadataAcess
	authorizer common.Authorizer
}

// createAPI returns a new image metadata API facade.
func createAPI(
	st metadataAcess,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() && !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}

	return &API{
		metadata:   st,
		authorizer: authorizer,
	}, nil
}

// NewAPI returns a new cloud image metadata API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	return createAPI(getState(st), resources, authorizer)
}

// List returns all found cloud image metadata that satisfy
// given filter.
// Returned list contains metadata for custom images first, then public.
func (api *API) List(filter params.ImageMetadataFilter) (params.ListCloudImageMetadataResult, error) {
	found, err := api.metadata.FindMetadata(cloudimagemetadata.MetadataFilter{
		Region:          filter.Region,
		Series:          filter.Series,
		Arches:          filter.Arches,
		Stream:          filter.Stream,
		VirtualType:     filter.VirtualType,
		RootStorageType: filter.RootStorageType,
	})
	if err != nil {
		return params.ListCloudImageMetadataResult{}, common.ServerError(err)
	}

	var all []params.CloudImageMetadata
	addAll := func(ms []cloudimagemetadata.Metadata) {
		for _, m := range ms {
			all = append(all, parseMetadataToParams(m))
		}
	}

	// First return metadata for custom images, then public.
	// No other source for cloud images should exist at the moment.
	// Once new source is identified, the order of returned metadata
	// may need to be changed.
	addAll(found[cloudimagemetadata.Custom])
	addAll(found[cloudimagemetadata.Public])

	return params.ListCloudImageMetadataResult{Result: all}, nil
}

// Save stores given cloud image metadata.
// It supports bulk calls.
func (api *API) Save(metadata params.MetadataSaveParams) (params.ErrorResults, error) {
	all := make([]params.ErrorResult, len(metadata.Metadata))
	for i, one := range metadata.Metadata {
		err := api.metadata.SaveMetadata(parseMetadataFromParams(one))
		all[i] = params.ErrorResult{Error: common.ServerError(err)}
	}
	return params.ErrorResults{Results: all}, nil
}

func parseMetadataToParams(p cloudimagemetadata.Metadata) params.CloudImageMetadata {
	result := params.CloudImageMetadata{
		ImageId:         p.ImageId,
		Stream:          p.Stream,
		Region:          p.Region,
		Series:          p.Series,
		Arch:            p.Arch,
		VirtualType:     p.VirtualType,
		RootStorageType: p.RootStorageType,
		RootStorageSize: p.RootStorageSize,
		Source:          string(p.Source),
	}
	return result
}

func parseMetadataFromParams(p params.CloudImageMetadata) cloudimagemetadata.Metadata {

	parseSource := func(s string) cloudimagemetadata.SourceType {
		switch cloudimagemetadata.SourceType(strings.ToLower(s)) {
		case cloudimagemetadata.Public:
			return cloudimagemetadata.Public
		case cloudimagemetadata.Custom:
			return cloudimagemetadata.Custom
		default:
			panic(fmt.Sprintf("unknown cloud image metadata source %q", s))
		}
	}

	result := cloudimagemetadata.Metadata{
		cloudimagemetadata.MetadataAttributes{
			Stream:          p.Stream,
			Region:          p.Region,
			Series:          p.Series,
			Arch:            p.Arch,
			VirtualType:     p.VirtualType,
			RootStorageType: p.RootStorageType,
			RootStorageSize: p.RootStorageSize,
			Source:          parseSource(p.Source),
		},
		p.ImageId,
	}
	if p.Stream == "" {
		result.Stream = "released"
	}
	return result
}
