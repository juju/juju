// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/series"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/imagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	envmetadata "github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = loggo.GetLogger("juju.apiserver.imagemetadata")

// API is the concrete implementation of the api end point
// for loud image metadata manipulations.
type API struct {
	metadata            metadataAccess
	newEnviron          func() (environs.Environ, error)
	imageSourceRegistry *environs.ImageSourceRegistry
}

// createAPI returns a new image metadata API facade.
func createAPI(
	st metadataAccess,
	newEnviron func() (environs.Environ, error),
	resources facade.Resources,
	authorizer facade.Authorizer,
	imageSourceRegistry *environs.ImageSourceRegistry,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	return &API{
		metadata:            st,
		newEnviron:          newEnviron,
		imageSourceRegistry: imageSourceRegistry,
	}, nil
}

// NewAPI returns a new cloud image metadata API facade.
func NewAPI(ctx facade.Context) (*API, error) {
	st := ctx.State()
	providerRegistry := ctx.ProviderRegistry()
	newEnviron := func() (environs.Environ, error) {
		return stateenvirons.GetNewEnvironFunc(providerRegistry.NewEnviron)(st)
	}
	return createAPI(getState(st), newEnviron, ctx.Resources(), ctx.Auth(), ctx.ImageSourceRegistry())
}

// UpdateFromPublishedImages retrieves currently published image metadata and
// updates stored ones accordingly.
func (api *API) UpdateFromPublishedImages() error {
	return api.retrievePublished()
}

func (api *API) retrievePublished() error {
	env, err := api.newEnviron()
	if err != nil {
		return errors.Annotatef(err, "getting environ")
	}

	sources, err := api.imageSourceRegistry.Sources(env)
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
	errs, err := imagecommon.Save(api.metadata, metadata)
	if err != nil {
		return errors.Annotatef(err, "saving published images metadata")
	}

	return processErrors(append(errs, parseErrs...))
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
