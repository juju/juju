// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/resources"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/resource"
)

// GetApplicationResourceID returns the ID of the application resource
// specified by natural key of application and resource name.
func (st *State) GetApplicationResourceID(
	ctx context.Context,
	args resource.GetApplicationResourceIDArgs,
) (resources.ID, error) {
	return "", nil
}

// ListResources returns the list of resources for the given application.
func (st *State) ListResources(
	ctx context.Context,
	applicationID application.ID,
) (resource.ApplicationResources, error) {
	return resource.ApplicationResources{}, nil
}

// GetResource returns the identified resource.
func (st *State) GetResource(ctx context.Context, resourceID resources.ID) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetResource adds the resource to blob storage and updates the metadata.
func (st *State) SetResource(
	ctx context.Context,
	config resource.SetResourceArgs,
) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetUnitResource sets the resource metadata for a specific unit.
func (st *State) SetUnitResource(
	ctx context.Context,
	config resource.SetUnitResourceArgs,
) (resource.SetUnitResourceResult, error) {
	return resource.SetUnitResourceResult{}, nil
}

// OpenApplicationResource returns the metadata for a resource.
func (st *State) OpenApplicationResource(
	ctx context.Context,
	resourceID resources.ID,
) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// OpenUnitResource returns the metadata for a resource. A unit
// resource is created to track the given unit and which resource
// its using.
func (st *State) OpenUnitResource(
	ctx context.Context,
	resourceID resources.ID,
	unitID coreunit.UUID,
) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetRepositoryResources sets the "polled" resource
// s for the
// application to the provided values. The current data for this
// application/resource combination will be overwritten.
func (st *State) SetRepositoryResources(
	ctx context.Context,
	config resource.SetRepositoryResourcesArgs,
) error {
	return nil
}
