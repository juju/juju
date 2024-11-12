// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/resources"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/resource"
)

// ResourceState is used to access the database.
type ResourceState struct {
	*commonStateBase
	logger logger.Logger
}

// NewResourceState creates a state to access the database.
func NewResourceState(factory database.TxnRunnerFactory, logger logger.Logger) *ResourceState {
	return &ResourceState{
		commonStateBase: &commonStateBase{
			StateBase: domain.NewStateBase(factory),
		},
		logger: logger,
	}
}

// GetApplicationResourceID returns the ID of the application resource
// specified by natural key of application and resource name.
func (st *ResourceState) GetApplicationResourceID(ctx context.Context, args resource.GetApplicationResourceIDArgs) (resources.ID, error) {
	return "", nil
}

// ListResources returns the list of resources for the given application.
func (st *ResourceState) ListResources(ctx context.Context, applicationID application.ID) (resource.ApplicationResources, error) {
	return resource.ApplicationResources{}, nil
}

// GetResource returns the identified resource.
func (st *ResourceState) GetResource(ctx context.Context, resourceID resources.ID) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetResource adds the resource to blob storage and updates the metadata.
func (st *ResourceState) SetResource(ctx context.Context, config resource.SetResourceArgs) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetUnitResource sets the resource metadata for a specific unit.
func (st *ResourceState) SetUnitResource(ctx context.Context, config resource.SetUnitResourceArgs) (resource.SetUnitResourceResult, error) {
	return resource.SetUnitResourceResult{}, nil
}

// OpenApplicationResource returns the metadata for a resource.
func (st *ResourceState) OpenApplicationResource(ctx context.Context, resourceID resources.ID) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// OpenUnitResource returns the metadata for a resource. A unit
// resource is created to track the given unit and which resource
// its using.
func (st *ResourceState) OpenUnitResource(ctx context.Context, resourceID resources.ID, unitID coreunit.UUID) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetRepositoryResources sets the "polled" resources for the
// application to the provided values. The current data for this
// application/resource combination will be overwritten.
func (st *ResourceState) SetRepositoryResources(ctx context.Context, config resource.SetRepositoryResourcesArgs) error {
	return nil
}
