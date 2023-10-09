// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package imagerepo

import (
	"github.com/juju/errors"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
)

// RegistryFunc is a function that can be used to create a Registry.
type RegistryFunc func(docker.ImageRepoDetails) (Registry, error)

// Registry is an interface that can be used to interact with a docker registry.
type Registry interface {
	// Ping checks that the registry is accessible.
	Ping() error
	// ImageRepoDetails returns the details of the image repo.
	ImageRepoDetails() docker.ImageRepoDetails
	// Close closes the registry.
	Close() error
}

// Option is a function that can be used to configure an ImageRepo.
type Option func(*option)

// WithRegistry sets the registry function to use when creating a Registry.
// This can be used for testing, or to provide additional logging/tracing to
// the registry.
func WithRegistry(regFunc RegistryFunc) Option {
	return func(o *option) {
		o.registryFunc = regFunc
	}
}

type option struct {
	registryFunc RegistryFunc
}

func newOption() *option {
	return &option{
		registryFunc: func(details docker.ImageRepoDetails) (Registry, error) {
			return registry.New(details)
		},
	}
}

// ImageRepo represents a docker image repository.
type ImageRepo struct {
	path       string
	details    *docker.ImageRepoDetails
	registryFn RegistryFunc
}

// NewImageRepo returns a new ImageRepo.
func NewImageRepo(path string, opts ...Option) (*ImageRepo, error) {
	if path == "" {
		return nil, errors.NotValidf("path")
	}

	o := newOption()
	for _, opt := range opts {
		opt(o)
	}

	details, err := docker.NewImageRepoDetails(path)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create image repo details")
	}
	if err := details.Validate(); err != nil {
		return nil, errors.Annotatef(err, "cannot validate image repo details")
	}
	return &ImageRepo{
		path:       path,
		details:    details,
		registryFn: o.registryFunc,
	}, nil
}

// Path returns the path to the image repo.
func (r *ImageRepo) Path() string {
	return r.path
}

// Ping checks that the image repo is accessible.
func (r *ImageRepo) Ping() error {
	return run(r.registryFn, r.details, func(reg Registry) error {
		return reg.Ping()
	})
}

// RawCredentials returns the raw credentials for the image repo.
// Note: this has the side effect of calling the registry to check
// that it is accessible and to return the credentials.
func (r *ImageRepo) RequestDetails() (docker.ImageRepoDetails, error) {
	var details docker.ImageRepoDetails
	err := run(r.registryFn, r.details, func(reg Registry) error {
		if err := reg.Ping(); err != nil {
			return errors.Annotatef(err, "cannot ping registry")
		}
		details = reg.ImageRepoDetails()
		return nil
	})
	return details, errors.Trace(err)
}

func run(regFunc RegistryFunc, details *docker.ImageRepoDetails, fn func(Registry) error) error {
	reg, err := regFunc(*details)
	if err != nil {
		return errors.Annotatef(err, "cannot create registry")
	}
	defer func() { _ = reg.Close() }()

	return fn(reg)
}

// DetailsFromPath returns the details of the image repo at the given path.
// This should only be used with image repo data after it has been validated.
func DetailsFromPath(path string) (docker.ImageRepoDetails, error) {
	if path == "" {
		return docker.ImageRepoDetails{}, nil
	}

	details, err := docker.NewImageRepoDetails(path)
	if err != nil {
		return docker.ImageRepoDetails{}, errors.Annotatef(err, "cannot create image repo details")
	}
	return *details, nil
}
