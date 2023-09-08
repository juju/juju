// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
)

// CloudConfigSchemaProvider is responsible for providing the ConfigSchemaSource
// for a cloud. If no ConfigSchemaSource is found for the provided cloud type
// then an error that satisfies NotFound is returned.
type CloudConfigSchemaProvider func(string) (config.ConfigSchemaSource, error)

// State describes retrieval and persistence methods for storage.
type State interface {
	// UpsertCloud persists the input cloud entity.
	UpsertCloud(context.Context, cloud.Cloud) error

	// DeleteCloud deletes the input cloud entity.
	DeleteCloud(context.Context, string) error

	// ListClouds returns the clouds matching the optional filter.
	ListClouds(context.Context, string) ([]cloud.Cloud, error)
}

// Service provides the API for working with clouds.
type Service struct {
	cloudConfigSchema CloudConfigSchemaProvider
	st                State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, cloudConfigSchema CloudConfigSchemaProvider) *Service {
	return &Service{
		cloudConfigSchema: cloudConfigSchema,
		st:                st,
	}
}

// Save inserts or updates the specified cloud.
func (s *Service) Save(ctx context.Context, cloud cloud.Cloud) error {
	err := s.st.UpsertCloud(ctx, cloud)
	return errors.Annotatef(err, "updating cloud %q", cloud.Name)
}

func (s *Service) Delete(ctx context.Context, name string) error {
	err := s.st.DeleteCloud(ctx, name)
	return errors.Annotatef(err, "deleting cloud %q", name)
}

// ListAll returns all the clouds.
func (s *Service) ListAll(ctx context.Context) ([]cloud.Cloud, error) {
	all, err := s.st.ListClouds(ctx, "")
	return all, errors.Trace(err)
}

// Get returns the named cloud.
func (s *Service) Get(ctx context.Context, name string) (*cloud.Cloud, error) {
	clouds, err := s.st.ListClouds(ctx, name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(clouds) == 0 {
		return nil, errors.NotFoundf("cloud %q", name)
	}
	result := clouds[0]
	return &result, nil
}
