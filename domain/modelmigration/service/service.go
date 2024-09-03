// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/version/v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/environs/envcontext"
	environscontext "github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
)

// InstanceProvider describes the interface that is needed from the cloud provider to
// implement the model migration service.
type InstanceProvider interface {
	AllInstances(envcontext.ProviderCallContext) ([]instances.Instance, error)
}

// ResourceProvider describes a provider for managing cloud resources on behalf
// of a model.
type ResourceProvider interface {
	// AdoptResources is called when the model is moved from one
	// controller to another using model migration. Some providers tag
	// instances, disks, and cloud storage with the controller UUID to
	// aid in clean destruction. This method will be called on the
	// environ for the target controller so it can update the
	// controller tags for all of those things. For providers that do
	// not track the controller UUID, a simple method returning nil
	// will suffice. The version number of the source controller is
	// provided for backwards compatibility - if the technique used to
	// tag items changes, the version number can be used to decide how
	// to remove the old tags correctly.
	AdoptResources(envcontext.ProviderCallContext, string, version.Number) error
}

// Service provides the means for supporting model migration actions between
// controllers and answering questions about the underlying model(s) that are
// being migrated.
type Service struct {
	// instanceProviderGetter is a getter for getting access to the model's
	// [InstanceProvider].
	instanceProviderGetter func(context.Context) (InstanceProvider, error)

	// resourceProviderGetter is a getter for getting access to the model's
	// [ResourceProvider]
	resourceProviderGettter func(context.Context) (ResourceProvider, error)

	st State
}

// State defines the interface required for accessing the underlying state of
// the model during migration.
type State interface {
	GetControllerUUID(context.Context) (string, error)
}

// NewService is responsible for constructing a new [Service] to handle model migration
// tasks.
func NewService(
	instanceProviderGetter providertracker.ProviderGetter[InstanceProvider],
	resourceProviderGetter providertracker.ProviderGetter[ResourceProvider],
	st State,
) *Service {
	return &Service{
		instanceProviderGetter:  instanceProviderGetter,
		resourceProviderGettter: resourceProviderGetter,
		st:                      st,
	}
}

// AdoptResources is responsible for taking ownership of the cloud resources of
// a model when it has been migrated into this controller.
func (s *Service) AdoptResources(
	ctx context.Context,
	sourceControllerVersion version.Number,
) error {
	provider, err := s.resourceProviderGettter(ctx)

	// Provider doesn't support adopting resources and this is ok!
	if errors.Is(err, coreerrors.NotSupported) {
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting resource provider for adopting model cloud resources: %w",
			err,
		)
	}

	controllerUUID, err := s.st.GetControllerUUID(ctx)
	if err != nil {
		return errors.Errorf(
			"cannot get controller uuid while adopting model cloud resources: %w",
			err,
		)
	}

	err = provider.AdoptResources(
		environscontext.WithoutCredentialInvalidator(ctx),
		controllerUUID,
		sourceControllerVersion,
	)

	// Provider doesn't support adopting resources and this is ok!
	if errors.Is(err, coreerrors.NotImplemented) {
		return nil
	}
	if err != nil {
		return errors.Errorf("cannot adopt cloud resources for model: %w", err)
	}
	return nil
}

// CheckMachines is responsible for checking a model after it has been migrated
// into this target controller. We check the machines that exist in the model
// against the machines reported by the models cloud and report any
// discrepancies.
func (s *Service) CheckMachines(
	ctx context.Context,
) ([]modelmigration.MigrationMachineDiscrepancy, error) {
	provider, err := s.instanceProviderGetter(ctx)
	if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return nil, errors.Errorf(
			"cannot get provider for model when checking for machine discrepancies in migrated model: %w",
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

	// TODO(instancedata) (tlm) 28/7/2024: This function is incomplete at the moment till we
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
