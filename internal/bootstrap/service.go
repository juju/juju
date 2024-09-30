// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
)

// ApplicationService instances create an application.
type ApplicationService interface {
	CreateApplication(context.Context, string, charm.Charm, corecharm.Origin, applicationservice.AddApplicationArgs, ...applicationservice.AddUnitArg) (coreapplication.ID, error)
	UpdateCAASUnit(ctx context.Context, unitName string, params applicationservice.UpdateCAASUnitParams) error
	SetUnitPassword(ctx context.Context, unitName string, passwordHash string) error
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}
