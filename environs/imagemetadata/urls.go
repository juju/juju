// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/simplestreams"
)

// SupportsCustomSources represents an environment that
// can host image metadata at provider specific sources.
type SupportsCustomSources interface {
	GetImageSources() ([]simplestreams.DataSource, error)
}

// GetMetadataSources returns the sources to use when looking for
// simplestreams image id metadata. If env implements
// SupportsCustomSurces, the sources returned from that method will also
// be considered.
func GetMetadataSources(env environs.ConfigGetter) ([]simplestreams.DataSource, error) {
	var sources []simplestreams.DataSource
	if userURL, ok := env.Config().ImageMetadataURL(); ok {
		sources = append(sources, simplestreams.NewURLDataSource(userURL, simplestreams.VerifySSLHostnames))
	}
	if custom, ok := env.(SupportsCustomSources); ok {
		customSources, err := custom.GetImageSources()
		if err != nil {
			return nil, err
		}
		sources = append(sources, customSources...)
	}

	if DefaultBaseURL != "" {
		sources = append(sources, simplestreams.NewURLDataSource(DefaultBaseURL, simplestreams.VerifySSLHostnames))
	}
	return sources, nil
}
