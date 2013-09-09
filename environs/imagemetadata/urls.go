// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/simplestreams"
)

// SupportsCustomSources instances can host image metadata at provider specific sources.
type SupportsCustomSources interface {
	GetImageSources() ([]simplestreams.DataSource, error)
}

// GetMetadataSources returns the sources to use when looking for simplestreams image id metadata.
func GetMetadataSources(cloudInst config.HasConfig) ([]simplestreams.DataSource, error) {
	var sources []simplestreams.DataSource
	if userURL, ok := cloudInst.Config().ImageMetadataURL(); ok {
		sources = append(sources, simplestreams.NewHttpDataSource(userURL))
	}
	if custom, ok := cloudInst.(SupportsCustomSources); ok {
		customSources, err := custom.GetImageSources()
		if err != nil {
			return nil, err
		}
		sources = append(sources, customSources...)
	}

	if DefaultBaseURL != "" {
		sources = append(sources, simplestreams.NewHttpDataSource(DefaultBaseURL))
	}
	return sources, nil
}
