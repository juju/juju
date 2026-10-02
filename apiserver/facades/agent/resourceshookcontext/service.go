// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
)

// ResourceService provides methods for managing resource data related
// to applications.
type ResourceService interface {

	// GetResourcesByApplicationUUID retrieves all resources associated with a
	// given application UUID in the specified context.
	GetResourcesByApplicationUUID(ctx context.Context, applicationID coreapplication.UUID) ([]coreresource.Resource,
		error)

	// GetUnitResourceID returns the UUID of the resource selected by a unit for
	// a logical resource name.
	GetUnitResourceID(ctx context.Context, args resource.GetUnitResourceIDArgs) (coreresource.UUID, error)

	// GetResourceWithoutApplication returns a resource without requiring a
	// current application link.
	GetResourceWithoutApplication(ctx context.Context, resourceUUID coreresource.UUID) (coreresource.Resource, error)
}

// ApplicationService defines operations to retrieve application UUIDs based
// on application or unit names.
type ApplicationService interface {
	// GetApplicationUUIDByName returns an application UUID by application name.
	// It returns an error if the application can not be found by the name.
	GetApplicationUUIDByName(ctx context.Context, name string) (coreapplication.UUID, error)

	// GetApplicationUUIDByUnitName returns the application UUID for the named
	// unit. It returns an error if the unit is not found by the name
	GetApplicationUUIDByUnitName(ctx context.Context, unitName coreunit.Name) (coreapplication.UUID, error)
}
