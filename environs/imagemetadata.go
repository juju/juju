// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
)

type datasourceFuncId struct {
	id string
	f  ImageDataSourceFunc
}

var (
	datasourceFuncsMu sync.RWMutex
	datasourceFuncs   []datasourceFuncId
)

// ImageDataSourceFunc is a function type that takes an environment and
// returns a simplestreams datasource.
//
// ImageDataSourceFunc will be used in ImageMetadataSources.
// Any error satisfying errors.IsNotSupported will be ignored;
// any other error will be cause ImageMetadataSources to fail.
type ImageDataSourceFunc func(Environ) (simplestreams.DataSource, error)

// RegisterImageDataSourceFunc registers an ImageDataSourceFunc
// with the specified id, overwriting any function previously registered
// with the same id.
func RegisterImageDataSourceFunc(id string, f ImageDataSourceFunc) {
	datasourceFuncsMu.Lock()
	defer datasourceFuncsMu.Unlock()
	for i := range datasourceFuncs {
		if datasourceFuncs[i].id == id {
			datasourceFuncs[i].f = f
			return
		}
	}
	datasourceFuncs = append(datasourceFuncs, datasourceFuncId{id, f})
}

// UnregisterImageDataSourceFunc unregisters an ImageDataSourceFunc
// with the specified id.
func UnregisterImageDataSourceFunc(id string) {
	for i, f := range datasourceFuncs {
		if f.id == id {
			head := datasourceFuncs[:i]
			tail := datasourceFuncs[i+1:]
			datasourceFuncs = append(head, tail...)
			return
		}
	}
}

// ImageMetadataSources returns the sources to use when looking for
// simplestreams image id metadata for the given stream.
func ImageMetadataSources(env Environ) ([]simplestreams.DataSource, error) {
	config := env.Config()

	// Add configured and environment-specific datasources.
	var sources []simplestreams.DataSource
	if userURL, ok := config.ImageMetadataURL(); ok {
		verify := utils.VerifySSLHostnames
		if !config.SSLHostnameVerification() {
			verify = utils.NoVerifySSLHostnames
		}
		sources = append(sources, simplestreams.NewURLDataSource("image-metadata-url", userURL, verify))
	}

	envDataSources, err := environmentDataSources(env)
	if err != nil {
		return nil, err
	}
	sources = append(sources, envDataSources...)

	// Add the default, public datasource.
	defaultURL, err := imagemetadata.ImageMetadataURL(imagemetadata.DefaultBaseURL, config.ImageStream())
	if err != nil {
		return nil, err
	}
	if defaultURL != "" {
		sources = append(sources,
			simplestreams.NewURLDataSource("default cloud images", defaultURL, utils.VerifySSLHostnames))
	}
	return sources, nil
}

// environmentDataSources returns simplestreams datasources for the environment
// by calling the functions registered in RegisterImageDataSourceFunc.
// The datasources returned will be in the same order the functions were registered.
func environmentDataSources(env Environ) ([]simplestreams.DataSource, error) {
	datasourceFuncsMu.RLock()
	defer datasourceFuncsMu.RUnlock()
	var datasources []simplestreams.DataSource
	for _, f := range datasourceFuncs {
		logger.Debugf("trying datasource %q", f.id)
		datasource, err := f.f(env)
		if err != nil {
			if errors.IsNotSupported(err) {
				continue
			}
			return nil, err
		}
		datasources = append(datasources, datasource)
	}
	return datasources, nil
}
