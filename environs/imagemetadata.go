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
	GlobalRegistry().ImageSources().RegisterFirst(id, f)
}

// RegisterImageDataSourceFunc registers an ImageDataSourceFunc
// with the specified id, overwriting any function previously registered
// with the same id.
func RegisterImageDataSourceFunc(id string, f ImageDataSourceFunc) {
	GlobalRegistry().ImageSources().Register(id, f)
}

// ImageMetadataSources returns the sources to use when looking for
// simplestreams image id metadata for the given Environ.
func ImageMetadataSources(env Environ) ([]simplestreams.DataSource, error) {
	return GlobalRegistry().ImageSources().Sources(env)
}

// UnregisterImageDataSourceFunc unregisters an ImageDataSourceFunc
// with the specified id.
func UnregisterImageDataSourceFunc(id string) {
	GlobalRegistry().ImageSources().Unregister(id)
}

type datasourceFuncId struct {
	id string
	f  ImageDataSourceFunc
}

// ImageSourceRegistry holds a slice of functions that
// map from Environ to image source.
type ImageSourceRegistry struct {
	mu    sync.RWMutex
	funcs []datasourceFuncId
}

func NewImageSourceRegistry() *ImageSourceRegistry {
	return &ImageSourceRegistry{}
}

// Register registers an ImageDataSourceFunc
// with the specified id, overwriting any function previously registered
// with the same id.
func (r *ImageSourceRegistry) Register(id string, f ImageDataSourceFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.funcs {
		if fid := &r.funcs[i]; fid.id == id {
			fid.f = f
			return
		}
	}
	logger.Debugf("new user image datasource registered: %v", id)
	r.funcs = append(r.funcs, datasourceFuncId{id, f})
}

// RegisterFirst registers an ImageDataSourceFunc
// with the specified id at the start of the search path, overwriting
// any function previously registered with the same id.
func (r *ImageSourceRegistry) RegisterFirst(id string, f ImageDataSourceFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.funcs {
		if fid := &r.funcs[i]; fid.id == id {
			fid.f = f
			return
		}
	}
	logger.Debugf("new user image datasource registered: %v", id)
	r.funcs = append([]datasourceFuncId{datasourceFuncId{id, f}}, r.funcs...)
}

// UnregisterImageDataSourceFunc unregisters an ImageDataSourceFunc
// with the specified id.
func (r *ImageSourceRegistry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, f := range r.funcs {
		if f.id == id {
			head := r.funcs[:i]
			tail := r.funcs[i+1:]
			r.funcs = append(head, tail...)
			return
		}
	}
}

// ImageMetadataSources returns the sources to use when looking for
// simplestreams image id metadata for the given Environ.
func (r *ImageSourceRegistry) Sources(env Environ) ([]simplestreams.DataSource, error) {
	config := env.Config()

	// Add configured and environment-specific datasources.
	var sources []simplestreams.DataSource
	if userURL, ok := config.ImageMetadataURL(); ok {
		verify := utils.VerifySSLHostnames
		if !config.SSLHostnameVerification() {
			verify = utils.NoVerifySSLHostnames
		}
		publicKey, _ := simplestreams.UserPublicSigningKey()
		sources = append(sources, simplestreams.NewURLSignedDataSource("image-metadata-url", userURL, publicKey, verify, simplestreams.SPECIFIC_CLOUD_DATA, false))
	}

	envDataSources, err := r.dataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sources = append(sources, envDataSources...)

	// Add the official image metadata datasources.
	officialDataSources, err := imagemetadata.OfficialDataSources(config.ImageStream())
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, source := range officialDataSources {
		sources = append(sources, source)
	}
	for _, ds := range sources {
		logger.Debugf("obtained image datasource %q", ds.Description())
	}
	return sources, nil
}

// dataSources returns simplestreams datasources for the environment
// by calling the registered functions.
// The datasources returned will be in the same order the functions were registered.
func (r *ImageSourceRegistry) dataSources(env Environ) ([]simplestreams.DataSource, error) {
	r.mu.Lock()
	funcs := r.funcs
	r.mu.Unlock()
	var datasources []simplestreams.DataSource
	for _, f := range funcs {
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
