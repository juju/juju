// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/config"
)

// ApplicationService instances create an application.
type ApplicationService interface {
	CreateApplication(context.Context, string, applicationservice.AddApplicationArgs, ...applicationservice.AddUnitArg) (coreapplication.ID, error)
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}
