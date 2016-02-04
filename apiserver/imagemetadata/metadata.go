// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/series"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envmetadata "github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
)

var logger = loggo.GetLogger("juju.apiserver.imagemetadata")

func init() {
	common.RegisterStandardFacade("ImageMetadata", 2, NewAPI)
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
	if !authorizer.AuthClient() && !authorizer.AuthModelManager() {
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
// Returned list contains metadata ordered by priority.
func (api *API) List(filter params.ImageMetadataFilter) (params.ListCloudImageMetadataResult, error) {
	found, err := api.metadata.FindMetadata(cloudimagemetadata.MetadataFilter{
		Region:          filter.Region,
		Series:          filter.Series,
		Arches:          filter.Arches,
		Stream:          filter.Stream,
		VirtType:        filter.VirtType,
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

	for _, ms := range found {
		addAll(ms)
	}
	sort.Sort(metadataList(all))

	return params.ListCloudImageMetadataResult{Result: all}, nil
}

// Save stores given cloud image metadata.
// It supports bulk calls.
func (api *API) Save(metadata params.MetadataSaveParams) (params.ErrorResults, error) {
	all := make([]params.ErrorResult, len(metadata.Metadata))
	envCfg, err := api.metadata.ModelConfig()
	if err != nil {
		return params.ErrorResults{}, errors.Annotatef(err, "getting environ config")
	}
	env, err := environs.New(envCfg)
	if err != nil {
		return params.ErrorResults{}, errors.Annotatef(err, "getting environ")
	}
	for i, one := range metadata.Metadata {
		md, err := api.parseMetadataListFromParams(one, env)
		if err != nil {
			all[i] = params.ErrorResult{Error: common.ServerError(err)}
			continue
		}
		err = api.metadata.SaveMetadata(md)
		all[i] = params.ErrorResult{Error: common.ServerError(err)}
	}
	return params.ErrorResults{Results: all}, nil
}

// Delete deletes cloud image metadata for given image ids.
// It supports bulk calls.
func (api *API) Delete(images params.MetadataImageIds) (params.ErrorResults, error) {
	all := make([]params.ErrorResult, len(images.Ids))
	for i, imageId := range images.Ids {
		err := api.metadata.DeleteMetadata(imageId)
		all[i] = params.ErrorResult{common.ServerError(err)}
	}
	return params.ErrorResults{Results: all}, nil
}

func parseMetadataToParams(p cloudimagemetadata.Metadata) params.CloudImageMetadata {
	result := params.CloudImageMetadata{
		ImageId:         p.ImageId,
		Stream:          p.Stream,
		Region:          p.Region,
		Version:         p.Version,
		Series:          p.Series,
		Arch:            p.Arch,
		VirtType:        p.VirtType,
		RootStorageType: p.RootStorageType,
		RootStorageSize: p.RootStorageSize,
		Source:          p.Source,
		Priority:        p.Priority,
	}
	return result
}

func (api *API) parseMetadataListFromParams(
	p params.CloudImageMetadataList, env environs.Environ,
) ([]cloudimagemetadata.Metadata, error) {
	results := make([]cloudimagemetadata.Metadata, len(p.Metadata))
	for i, metadata := range p.Metadata {
		result, err := api.parseMetadataFromParams(metadata, env)
		if err != nil {
			return nil, errors.Trace(err)
		}
		results[i] = result
	}
	return results, nil
}

func (api *API) parseMetadataFromParams(p params.CloudImageMetadata, env environs.Environ) (cloudimagemetadata.Metadata, error) {
	result := cloudimagemetadata.Metadata{
		cloudimagemetadata.MetadataAttributes{
			Stream:          p.Stream,
			Region:          p.Region,
			Version:         p.Version,
			Series:          p.Series,
			Arch:            p.Arch,
			VirtType:        p.VirtType,
			RootStorageType: p.RootStorageType,
			RootStorageSize: p.RootStorageSize,
			Source:          p.Source,
		},
		p.Priority,
		p.ImageId,
	}

	// Fill in any required default values.
	if p.Stream == "" {
		result.Stream = env.Config().ImageStream()
	}
	if p.Source == "" {
		result.Source = "custom"
	}
	if result.Arch == "" {
		result.Arch = "amd64"
	}
	if result.Series == "" {
		result.Series = config.PreferredSeries(env.Config())
	}
	if result.Region == "" {
		// If the env supports regions, use the env default.
		if r, ok := env.(simplestreams.HasRegion); ok {
			spec, err := r.Region()
			if err != nil {
				return cloudimagemetadata.Metadata{}, errors.Annotatef(err, "getting cloud region")
			}
			result.Region = spec.Region
		}
	}
	return result, nil
}

// UpdateFromPublishedImages retrieves currently published image metadata and
// updates stored ones accordingly.
func (api *API) UpdateFromPublishedImages() error {
	return api.retrievePublished()
}

func (api *API) retrievePublished() error {
	envCfg, err := api.metadata.ModelConfig()
	if err != nil {
		return errors.Annotatef(err, "getting environ config")
	}
	env, err := environs.New(envCfg)
	if err != nil {
		return errors.Annotatef(err, "getting environ")
	}

	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return errors.Annotatef(err, "getting cloud specific image metadata sources")
	}

	cons := envmetadata.NewImageConstraint(simplestreams.LookupParams{})
	if inst, ok := env.(simplestreams.HasRegion); !ok {
		// Published image metadata for some providers are in simple streams.
		// Providers that do not rely on simplestreams, don't need to do anything here.
		return nil
	} else {
		// If we can determine current region,
		// we want only metadata specific to this region.
		cloud, err := inst.Region()
		if err != nil {
			return errors.Annotatef(err, "getting cloud specific region information")
		}
		cons.CloudSpec = cloud
	}

	// We want all relevant metadata from all data sources.
	for _, source := range sources {
		logger.Debugf("looking in data source %v", source.Description())
		metadata, info, err := envmetadata.Fetch([]simplestreams.DataSource{source}, cons)
		if err != nil {
			// Do not stop looking in other data sources if there is an issue here.
			logger.Errorf("encountered %v while getting published images metadata from %v", err, source.Description())
			continue
		}
		err = api.saveAll(info, source.Priority(), metadata)
		if err != nil {
			// Do not stop looking in other data sources if there is an issue here.
			logger.Errorf("encountered %v while saving published images metadata from %v", err, source.Description())
		}
	}
	return nil
}

func (api *API) saveAll(info *simplestreams.ResolveInfo, priority int, published []*envmetadata.ImageMetadata) error {
	metadata, parseErrs := convertToParams(info, priority, published)

	// Store converted metadata.
	// Note that whether the metadata actually needs
	// to be stored will be determined within this call.
	errs, err := api.Save(metadata)
	if err != nil {
		return errors.Annotatef(err, "saving published images metadata")
	}

	return processErrors(append(errs.Results, parseErrs...))
}

// convertToParams converts model-specific images metadata to structured metadata format.
var convertToParams = func(info *simplestreams.ResolveInfo, priority int, published []*envmetadata.ImageMetadata) (params.MetadataSaveParams, []params.ErrorResult) {
	metadata := []params.CloudImageMetadataList{{}}
	errs := []params.ErrorResult{}
	for _, p := range published {
		s, err := series.VersionSeries(p.Version)
		if err != nil {
			errs = append(errs, params.ErrorResult{Error: common.ServerError(err)})
			continue
		}

		m := params.CloudImageMetadata{
			Source:          info.Source,
			ImageId:         p.Id,
			Stream:          p.Stream,
			Region:          p.RegionName,
			Arch:            p.Arch,
			VirtType:        p.VirtType,
			RootStorageType: p.Storage,
			Series:          s,
			Priority:        priority,
		}

		metadata[0].Metadata = append(metadata[0].Metadata, m)
	}
	return params.MetadataSaveParams{Metadata: metadata}, errs
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
