// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package imagerepo

import (
	"github.com/juju/errors"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
)

// ImageRepo represents a docker image repository.
type ImageRepo struct {
	path    string
	details *docker.ImageRepoDetails
}

// NewImageRepo returns a new ImageRepo.
func NewImageRepo(path string) (*ImageRepo, error) {
	if path == "" {
		return nil, errors.NotValidf("path")
	}
	details, err := docker.NewImageRepoDetails(path)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create image repo details")
	}
	if err := details.Validate(); err != nil {
		return nil, errors.Annotatef(err, "cannot validate image repo details")
	}
	return &ImageRepo{
		path:    path,
		details: details,
	}, nil
}

// Path returns the path to the image repo.
func (r *ImageRepo) Path() string {
	return r.path
}

// Ping checks that the image repo is accessible.
func (r *ImageRepo) Ping() error {
	return run(r.details, func(reg registry.Registry) error {
		return reg.Ping()
	})
}

// RawCredentials returns the raw credentials for the image repo.
// Note: this has the side effect of calling the registry to check
// that it is accessible and to return the credentials.
func (r *ImageRepo) RequestDetails() (docker.ImageRepoDetails, error) {
	var details docker.ImageRepoDetails
	err := run(r.details, func(reg registry.Registry) error {
		if err := reg.Ping(); err != nil {
			return errors.Annotatef(err, "cannot ping registry")
		}
		details = reg.ImageRepoDetails()
		return nil
	})
	return details, errors.Trace(err)
}

func run(details *docker.ImageRepoDetails, fn func(registry.Registry) error) error {
	reg, err := registry.New(*details)
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
