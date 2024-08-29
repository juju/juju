// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
)

// Provider describes the interface that is needed from the cloud provider to
// implement the model migration service.
type Provider interface {
	AllInstances(ctx envcontext.ProviderCallContext) ([]instances.Instance, error)
}

// Service provides the means for supporting model migration actions between
// controllers and answering questions about the underlying model(s) that are
// being migrated.
type Service struct {
	// providerGetter is a getter for getting access to the models [Provider].
	providerGetter func(context.Context) (Provider, error)
}

// New is responsible for constructing a new [Service] to handle model migration
// tasks.
func New(providerGetter providertracker.ProviderGetter[Provider]) *Service {
	return &Service{
		providerGetter: providerGetter,
	}
}

// CheckMachines is responsible for checking a model after it has been migrated
// into this target controller. We check the machines that exist in the model
// against the machines reported by the models cloud and report any
// discrepancies.
func (s *Service) CheckMachines(
	ctx context.Context,
) ([]modelmigration.MigrationMachineDiscrepancy, error) {
	provider, err := s.providerGetter(ctx)
	if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return nil, errors.Errorf(
			"cannot get provider for model to check machines provider machines against the controller: %w",
			err,
		)
	}

	// If the provider doesn't support machines we can bail out early.
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, nil
	}

	_, err = provider.AllInstances(envcontext.WithoutCredentialInvalidator(ctx))
	if err != nil {
		return nil, errors.Errorf(
			"cannot get all provider instances for model when checking machines: %w",
			err,
		)
	}

	// TODO (tlm) 28/7/2024: This function is incomplete at the moment till we
	// fully have machines/instance data moved over into Dqlite.
	//
	// The algorithm we need to implement here is:
	// 1. Get all machines for the model and build a mapping of the machines
	// based on instance id to machine name.
	// 2. Get a list of all the instances from the provider.
	// 3. Build a set for the instance ids from the provider and while iterating
	// over each instance id check to see if we have a machine that is using this
	// instance id. If we don't have a machine this is a discrepancy.
	// 4. For each machine we have in the model check to see the corresponding
	// instance id is in the set returned from the provider. If it doesn't exist
	// this is a discrepancy as well.

	return nil, nil
}
