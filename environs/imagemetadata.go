// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"
	"sync"

	"github.com/juju/errors"

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

// RegisterUserImageDataSourceFunc registers an ImageDataSourceFunc
// with the specified id at the start of the search path, overwriting
// any function previously registered with the same id.
func RegisterUserImageDataSourceFunc(id string, f ImageDataSourceFunc) {
	datasourceFuncsMu.Lock()
	defer datasourceFuncsMu.Unlock()
	for i := range datasourceFuncs {
		if datasourceFuncs[i].id == id {
			datasourceFuncs[i].f = f
			return
		}
	}
	logger.Debugf(context.Background(), "new user image datasource registered: %v", id)
	datasourceFuncs = append([]datasourceFuncId{{id, f}}, datasourceFuncs...)
}

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
	logger.Debugf(context.Background(), "new model image datasource registered: %v", id)
	datasourceFuncs = append(datasourceFuncs, datasourceFuncId{id, f})
}

// UnregisterImageDataSourceFunc unregisters an ImageDataSourceFunc
// with the specified id.
func UnregisterImageDataSourceFunc(id string) {
	datasourceFuncsMu.Lock()
	defer datasourceFuncsMu.Unlock()
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
func ImageMetadataSources(env BootstrapEnviron, dataSourceFactory simplestreams.DataSourceFactory) ([]simplestreams.DataSource, error) {
	config := env.Config()

	// Add configured and environment-specific datasources.
	var sources []simplestreams.DataSource
	if userURL, ok := config.ImageMetadataURL(); ok {
		publicKey, err := simplestreams.UserPublicSigningKey()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg := simplestreams.Config{
			Description:          "image-metadata-url",
			BaseURL:              userURL,
			PublicSigningKey:     publicKey,
			HostnameVerification: config.SSLHostnameVerification(),
			Priority:             simplestreams.SPECIFIC_CLOUD_DATA,
		}
		if err := cfg.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		dataSource := dataSourceFactory.NewDataSource(cfg)
		sources = append(sources, dataSource)
	}

	envDataSources, err := environmentDataSources(context.TODO(), env)
	if err != nil {
		return nil, err
	}
	sources = append(sources, envDataSources...)

	if config.ImageMetadataDefaultsDisabled() {
		logger.Debugf(context.TODO(), "default image metadata sources are disabled")
	} else {
		// Add the official image metadata datasources.
		officialDataSources, err := imagemetadata.OfficialDataSources(dataSourceFactory, config.ImageStream())
		if err != nil {
			return nil, err
		}
		sources = append(sources, officialDataSources...)
	}

	for _, ds := range sources {
		logger.Debugf(context.TODO(), "obtained image datasource %q", ds.Description())
	}
	return sources, nil
}

// environmentDataSources returns simplestreams datasources for the environment
// by calling the functions registered in RegisterImageDataSourceFunc.
// The datasources returned will be in the same order the functions were registered.
func environmentDataSources(ctx context.Context, bootstrapEnviron BootstrapEnviron) ([]simplestreams.DataSource, error) {
	datasourceFuncsMu.RLock()
	defer datasourceFuncsMu.RUnlock()
	var datasources []simplestreams.DataSource
	env, ok := bootstrapEnviron.(Environ)
	if !ok {
		logger.Debugf(ctx, "environmentDataSources is supported for IAAS, environ %#v is not Environ", bootstrapEnviron)
		// ignore for CAAS
		return datasources, nil
	}
	for _, f := range datasourceFuncs {
		datasource, err := f.f(env)
		if err != nil {
			if errors.Is(err, errors.NotSupported) {
				continue
			}
			return nil, err
		}
		datasources = append(datasources, datasource)
	}
	return datasources, nil
}
