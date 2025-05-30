// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
)

// AgentPasswordService provides access to agent password management.
type AgentPasswordService interface {
	// SetUnitPassword sets the password for the given unit. If the unit does
	// not exist, an error satisfying [applicationerrors.UnitNotFound] is
	// returned.
	SetUnitPassword(ctx context.Context, unitName unit.Name, password string) error
}

type ApplicationService interface {
	// ResolveControllerCharmDownload resolves the controller charm download
	// slot.
	ResolveControllerCharmDownload(
		ctx context.Context,
		resolve application.ResolveControllerCharmDownload,
	) (application.ResolvedControllerCharmDownload, error)

	// UpdateCloudService updates the cloud service for the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
	UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.ProviderAddresses) error
}

// IAASApplicationService instances create an IAAS application.
type IAASApplicationService interface {
	// CreateIAASApplication creates a new application with the given name and
	// charm.
	CreateIAASApplication(
		context.Context, string, charm.Charm, corecharm.Origin,
		applicationservice.AddApplicationArgs, ...applicationservice.AddUnitArg,
	) (coreapplication.ID, error)
}

// CAASApplicationService instances create an IAAS application.
type CAASApplicationService interface {
	// CreateCAASApplication creates a new application with the given name and
	// charm.
	CreateCAASApplication(
		context.Context, string, charm.Charm, corecharm.Origin,
		applicationservice.AddApplicationArgs, ...applicationservice.AddUnitArg,
	) (coreapplication.ID, error)

	// UpdateApplication updates the application with the given name.
	UpdateCAASUnit(ctx context.Context, unitName unit.Name, params applicationservice.UpdateCAASUnitParams) error

	// UpdateCloudService updates the cloud service for the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
	UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.ProviderAddresses) error
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}
