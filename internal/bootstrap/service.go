// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/relation"
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

// ApplicationService instances create an application.
type ApplicationService interface {
	// CreateApplication creates a new application with the given name and
	// charm.
	CreateApplication(
		context.Context, string, charm.Charm, corecharm.Origin,
		applicationservice.AddApplicationArgs, relation.CreatePeerRelationsFunc,
	...applicationservice.AddUnitArg,
	) (coreapplication.ID, error)

	// ResolveControllerCharmDownload resolves the controller charm download
	// slot.
	ResolveControllerCharmDownload(
		ctx context.Context,
		resolve application.ResolveControllerCharmDownload,
	) (application.ResolvedControllerCharmDownload, error)

	// UpdateApplication updates the application with the given name.
	UpdateCAASUnit(ctx context.Context, unitName unit.Name, params applicationservice.UpdateCAASUnitParams) error
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// RelationService defines operations for managing relations between application
// endpoints.
type RelationService interface {
	// CreatePeerRelations creates the peer relations for an application.
	CreatePeerRelations(ctx context.Context, appId coreapplication.ID) error
}
