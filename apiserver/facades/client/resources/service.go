// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/resource"
)

// ResourceService defines methods for managing application resources.
type ResourceService interface {
	// ListResources returns the resources for the given application.
	ListResources(ctx context.Context, applicationID coreapplication.ID) (resource.ApplicationResources, error)
}

// ApplicationService defines methods to manage application.
type ApplicationService interface {
	// GetApplicationIDByName returns an application ID by application name.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)
}
