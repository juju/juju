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
	// AddResourcesBeforeApplication stages resource revisions before an
	// application is created or its charm is changed. The resources are
	// activated by the application operation using the returned UUIDs.
	AddResourcesBeforeApplication(ctx context.Context, arg resource.AddResourcesBeforeApplicationArgs) ([]coreresource.UUID, error)

	// ListResources returns the resources for the given application.
	ListResources(ctx context.Context, applicationID coreapplication.UUID) (coreresource.ApplicationResources, error)
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
