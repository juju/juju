// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

type ApplicationService interface {

	// GetApplicationIDByName returns an application ID by application name. It
	// returns an error if the application can not be found by the name.
	//
	// Returns [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationIDByName(ctx context.Context, name string) (application.ID, error)
}

type StatusService interface {
	// GetApplicationDisplayStatus returns the display status of the specified application.
	// The display status is equal to the application status if it is set, otherwise it is
	// derived from the unit display statuses.
	// If no application is found, an error satisfying [applicationerrors.ApplicationNotFound]
	// is returned.
	GetApplicationDisplayStatus(context.Context, application.ID) (*status.StatusInfo, error)
}
