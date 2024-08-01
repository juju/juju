// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
)

// ApplicationService instances save an application to dqlite state.
type ApplicationService interface {
	CreateApplication(ctx context.Context, name string, charm charm.Charm, params applicationservice.AddApplicationArgs, units ...applicationservice.AddUnitArg) (coreapplication.ID, error)
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ModelService provides a means for interacting with the underlying models of
// this controller
type ModelService interface {
	// ControllerModel returns the representation of the model that is used for
	// running the Juju controller.
	// Should this model not have been established yet an error satisfying
	// [github.com/juju/juju/domain/model/errors.NotFound] will be returned.
	ControllerModel(context.Context) (model.Model, error)
}
