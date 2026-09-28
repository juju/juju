// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/application"
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
	ListResources(ctx context.Context, applicationID coreapplication.UUID) (coreresource.ApplicationResources, error)

	// UpdateResourceRevision adds a new entry for the revision in the resource
	// table with the desired parameters and sets it on the application. The old
	// immutable resource and its blob are retained. The new resource UUID is
	// returned.
	UpdateResourceRevision(ctx context.Context, args resource.UpdateResourceRevisionArgs) (coreresource.UUID, error)

	// UpdateUploadResource adds a new entry for an uploaded blob in the resource
	// table with the desired parameters and sets it on the application. The old
	// immutable resource and its blob are retained. The new resource UUID is
	// returned.
	UpdateUploadResource(ctx context.Context, resourceToUpdate coreresource.UUID) (coreresource.UUID, error)
}

// ApplicationService defines methods to manage application.
type ApplicationService interface {
	// GetApplicationDetailsByName returns the application details for the given
	// application name. This includes the UUID, life status, name, and whether
	// the application is synthetic.
	GetApplicationDetailsByName(ctx context.Context, name string) (application.ApplicationDetails, error)
}

// CrossModelRelationService provides access to the cross model relation service.
type CrossModelRelationService interface {
	// IsApplicationSynthetic checks if the given application exists in the model
	// and is a synthetic application.
	IsApplicationSynthetic(ctx context.Context, appName string) (bool, error)
}
