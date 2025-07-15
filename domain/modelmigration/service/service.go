// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
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
) error {
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
) ([]modelmigration.MigrationMachineDiscrepancy, error) {
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

// ModelMigrationMode returns the current migration mode for the model.
func (s *Service) ModelMigrationMode(ctx context.Context) (modelmigration.MigrationMode, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement migration mode reporting.
	return modelmigration.MigrationModeNone, nil
}

// Migration returns status about migration of this model.
func (s *Service) Migration(ctx context.Context) (modelmigration.Migration, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement migration info reporting.
	return modelmigration.Migration{
		Phase: migration.NONE,
	}, nil
}

// InitiateMigration kicks off migrating this model to the target controller.
func (s *Service) InitiateMigration(ctx context.Context, targetInfo migration.TargetInfo, userName string) (string, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement migration info reporting.
	return "", errors.ConstError("migration is not implemented")
}

// WatchForMigration returns a notification watcher that fires when this model
// undergoes migration.
func (s *Service) WatchForMigration(ctx context.Context) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement migration watcher.
	return watcher.TODO[struct{}](), nil
}

// WatchMigrationPhase returns a notification watcher that fires when this
// model's migration phase changes.
func (s *Service) WatchMigrationPhase(ctx context.Context) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement migration watcher.
	return watcher.TODO[struct{}](), nil
}

// ReportFromUnit accepts a phase report from a migration minion for a unit
// agent.
func (s *Service) ReportFromUnit(ctx context.Context, unitName unit.Name, phase migration.Phase) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement reporting phase from a unit.
	return errors.ConstError("migration report from a unit is not implemented")
}

// ReportFromMachine accepts a phase report from a migration minion for a
// machine agent.
func (s *Service) ReportFromMachine(ctx context.Context, machineName machine.Name, phase migration.Phase) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement reporting phase from a machine.
	return errors.ConstError("migration report from a machine is not implemented")
}

// SetMigrationPhase is called by the migration master to progress migration.
func (s *Service) SetMigrationPhase(ctx context.Context, phase migration.Phase) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement reporting phase from migration master.
	return errors.ConstError("setting migration phase is not implemented")
}

// SetMigrationStatusMessage is called by the migration master to report on
// migration status.
func (s *Service) SetMigrationStatusMessage(ctx context.Context, message string) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement setting migration status message.
	return errors.ConstError("setting migration status message is not implemented")
}

// WatchMinionReports returns a notification watcher that fires when any minion
// reports a update to their phase.
func (s *Service) WatchMinionReports(ctx context.Context) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement watching minion reports.
	return watcher.TODO[struct{}](), nil
}

// MinionReports returns phase information about minions in this model.
func (s *Service) MinionReports(ctx context.Context) (migration.MinionReports, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement getting minion reports.
	return migration.MinionReports{}, errors.ConstError("getting minion reports is not implemented")
}

// AbortImport stops the import of the model.
func (s *Service) AbortImport(ctx context.Context) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement aborting model import.
	return errors.ConstError("aborting the import of a model is not implemented")
}

// ActivateImport finalises the import of the model.
func (s *Service) ActivateImport(ctx context.Context) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	// TODO(modelmigration): implement activate imported model.
	return errors.ConstError("activating an imported model is not implemented")
}
