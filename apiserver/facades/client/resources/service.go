// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/resource"
)

// ResourceService defines methods for managing application resources.
type ResourceService interface {
	// AddResourcesBeforeApplication adds the details of which resource
	// revision to use before the application exists in the model. The
	// charm and resource metadata must exist. These resources are resolved
	// when the application is created using the returned Resource UUIDs.
	AddResourcesBeforeApplication(ctx context.Context, arg resource.AddResourcesBeforeApplicationArgs) ([]coreresource.UUID, error)

	// GetApplicationResourceID returns the ID of the application resource
	// specified by natural key of application and resource name.
	GetApplicationResourceID(
		ctx context.Context,
		args resource.GetApplicationResourceIDArgs,
	) (coreresource.UUID, error)

	// ListResources returns the resources for the given application.
	ListResources(ctx context.Context, applicationID coreapplication.ID) (coreresource.ApplicationResources, error)

	// UpdateResourceRevision updates the revision of a store resource to a new
	// version. Increments charm modified version for the application to
	// trigger use of the new resource revision by the application. To allow for
	// a resource upgrade, the current resource blob is removed.
	UpdateResourceRevision(ctx context.Context, args resource.UpdateResourceRevisionArgs) error
}

// ApplicationService defines methods to manage application.
type ApplicationService interface {
	// GetApplicationIDByName returns an application ID by application name.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)
}
