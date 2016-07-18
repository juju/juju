// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/cloudimagemetadata"
)

var (
	CreateAPI     = createAPI
	ProcessErrors = processErrors
)

func ParseMetadataFromParams(api *API, p params.CloudImageMetadata, cfg *config.Config, cloudRegion string) (cloudimagemetadata.Metadata, error) {
	return api.parseMetadataFromParams(p, cfg, cloudRegion)
}
