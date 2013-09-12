// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/simplestreams"
)

// SupportsCustomSources instances can host tools metadata at provider specific sources.
type SupportsCustomSources interface {
	GetToolsSources() ([]simplestreams.DataSource, error)
}

// GetMetadataSources returns the sources to use when looking for simplestreams tools metadata.
func GetMetadataSources(cloudInst config.HasConfig) ([]simplestreams.DataSource, error) {
	var sources []simplestreams.DataSource
	if userURL, ok := cloudInst.Config().ToolsURL(); ok {
		sources = append(sources, simplestreams.NewURLDataSource(userURL))
	}
	if custom, ok := cloudInst.(SupportsCustomSources); ok {
		customSources, err := custom.GetToolsSources()
		if err != nil {
			return nil, err
		}
		sources = append(sources, customSources...)
	}

	if DefaultBaseURL != "" {
		sources = append(sources, simplestreams.NewURLDataSource(DefaultBaseURL))
	}
	return sources, nil
}
