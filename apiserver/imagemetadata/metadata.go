// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"fmt"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/utils/series"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	envmetadata "github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
)

var logger = loggo.GetLogger("juju.apiserver.imagemetadata")

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

// UpdateFromPublishedImages retrieves currently published image metadata and
// updates stored ones accordingly.
func (api *API) UpdateFromPublishedImages() error {
	published, err := api.retrievePublished()
	if err != nil {
		return errors.Annotatef(err, "getting published images metadata")
	}
	err = api.saveAll(published)
	return errors.Annotatef(err, "saving published images metadata")
}

func (api *API) retrievePublished() ([]*envmetadata.ImageMetadata, error) {
	// Get environ
	envCfg, err := api.metadata.EnvironConfig()
	env, err := environs.New(envCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Get all images metadata sources for this environ.
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, err
	}

	// We want all metadata.
	cons := envmetadata.NewImageConstraint(simplestreams.LookupParams{})
	metadata, _, err := envmetadata.Fetch(sources, cons, false)
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

func (api *API) saveAll(published []*envmetadata.ImageMetadata) error {
	// Store converted metadata.
	// Note that whether the metadata actually needs
	// to be stored will be determined within this call.
	errs, err := api.Save(convertToParams(published))
	if err != nil {
		return errors.Annotatef(err, "saving published images metadata")
	}
	return processErrors(errs.Results)
}

// convertToParams converts environment-specific images metadata to structured metadata format.
var convertToParams = func(published []*envmetadata.ImageMetadata) params.MetadataSaveParams {
	metadata := make([]params.CloudImageMetadata, len(published))
	for i, p := range published {
		metadata[i] = params.CloudImageMetadata{
			Source:          "public",
			ImageId:         p.Id,
			Stream:          p.Stream,
			Region:          p.RegionName,
			Arch:            p.Arch,
			VirtualType:     p.VirtType,
			RootStorageType: p.Storage,
		}
		// Translate version (eg.14.04) to a series (eg. "trusty")
		metadata[i].Series = versionSeries(p.Version)
	}

	return params.MetadataSaveParams{Metadata: metadata}
}

// TODO(dfc) why is this like this ?
var seriesVersion = series.SeriesVersion

func versionSeries(v string) string {
	if v == "" {
		return v
	}
	for _, s := range series.SupportedSeries() {
		sv, err := seriesVersion(s)
		if err != nil {
			logger.Errorf("cannot determine version for series %v: %v", s, err)
		}
		if v == sv {
			return s
		}
	}
	return v
}

func processErrors(errs []params.ErrorResult) error {
	msgs := []string{}
	for _, e := range errs {
		if e.Error != nil && e.Error.Message != "" {
			msgs = append(msgs, e.Error.Message)
		}
	}
	if len(msgs) != 0 {
		return errors.Errorf("saving some image metadata:\n%v", strings.Join(msgs, "\n"))
	}
	return nil
}
