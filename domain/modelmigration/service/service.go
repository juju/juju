// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
)

// InstanceProvider describes the interface that is needed from the cloud provider to
// implement the model migration service.
type InstanceProvider interface {
	AllInstances(context.Context) ([]instances.Instance, error)
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
	AdoptResources(context.Context, string, semversion.Number) error
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
	// GetAllInstanceIDs returns all instance IDs from the current model as
	// juju/collections set.
	GetAllInstanceIDs(ctx context.Context) (set.Strings, error)
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
	sourceControllerVersion semversion.Number,
) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
		ctx,
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
) (_ []modelmigration.MigrationMachineDiscrepancy, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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

	providerInstances, err := provider.AllInstances(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"cannot get all provider instances for model when checking machines: %w",
			err,
		)
	}

	// Build the sets of provider instance IDs and model machine instance IDs.
	providerInstanceIDsSet := make(set.Strings, len(providerInstances))
	for _, instance := range providerInstances {
		providerInstanceIDsSet.Add(instance.Id().String())
	}

	instanceIDsSet, err := s.st.GetAllInstanceIDs(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get all instance IDs for model when checking machines: %w", err)
	}
	// First check that all the instance IDs in the model are in the provider.
	if difference := instanceIDsSet.Difference(providerInstanceIDsSet); difference.Size() > 0 {
		return nil, errors.Errorf("instance IDs %q are not part of the provider instance IDs", difference.Values())
	}
	// Then check that all the instance ids in the provider correspond to model
	// machines instance IDs
	if difference := providerInstanceIDsSet.Difference(instanceIDsSet); difference.Size() > 0 {
		return nil, errors.Errorf("provider instance IDs %q are not part of the model machines instance IDs", difference.Values())
	}

	return nil, nil
}
