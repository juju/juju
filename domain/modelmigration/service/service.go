// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
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
	// AdoptResources is called when the model is moved from one controller to
	// another using model migration. Some providers tag instances, disks, and
	// cloud storage with the controller UUID to aid in clean destruction. This
	// method will be called on the environ for the target controller so it can
	// update the controller tags for all of those things. For providers that do
	// not track the controller UUID, a simple method returning nil will
	// suffice. The version number of the source controller is provided for
	// backwards compatibility - if the technique used to tag items changes, the
	// version number can be used to decide how to remove the old tags
	// correctly.
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
	resourceProviderGetter func(context.Context) (ResourceProvider, error)

	controllerState ControllerState
	modelState      ModelState
	watcherFactory  WatcherFactory
	modelUUID       string
}

// WatcherFactory describes methods for creating watchers used by the
// [Service].
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required,
	// though additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// ControllerState defines the interface required for accessing the underlying
// state of the model during migration.
type ControllerState interface {
	// GetControllerTargetVersion returns the target controller version in use
	// by the cluster.
	GetControllerTargetVersion(ctx context.Context) (string, error)

	// DeleteModelImportingStatus removes the entry from the model_migrating
	// table in the model database, indicating that the model import has
	// completed or been aborted.
	DeleteModelImportingStatus(ctx context.Context, modelUUID string) error
}

// ModelState defines the interface required for accessing the underlying state
// of the model during migration.
type ModelState interface {
	// GetControllerUUID returns the UUID of the controller that owns this
	// model.
	GetControllerUUID(context.Context) (string, error)
	// GetAllInstanceIDs returns all instance IDs from the current model as
	// juju/collections set.
	GetAllInstanceIDs(ctx context.Context) (set.Strings, error)
	// GetModelTargetAgentVersion returns the target agent version for this
	// model.
	GetModelTargetAgentVersion(context.Context) (string, error)
	// SetModelTargetAgentVersion is responsible for setting the current target
	// agent version of the model. This function expects a precondition version
	// to be supplied. The model's target version at the time the operation is
	// applied must match the preCondition version or else an error is returned.
	SetModelTargetAgentVersion(
		ctx context.Context, preCondition, toVersion string,
	) error
	// DeleteModelImportingStatus removes the entry from the model_migrating
	// table in the model database, indicating that the model import has
	// completed or been aborted.
	DeleteModelImportingStatus(ctx context.Context) error

	// GetNamespaceModelMigrating returns the name of the model_migrating
	// changestream namespace. A change in this namespace indicates that this
	// model has started or stopped undergoing a migration.
	GetNamespaceModelMigrating() string
}

// NewService is responsible for constructing a new [Service] to handle model
// migration tasks.
func NewService(
	controllerState ControllerState,
	modelState ModelState,
	modelUUID string,
	watcherFactory WatcherFactory,
	instanceProviderGetter providertracker.ProviderGetter[InstanceProvider],
	resourceProviderGetter providertracker.ProviderGetter[ResourceProvider],
) *Service {
	return &Service{
		controllerState:        controllerState,
		modelState:             modelState,
		watcherFactory:         watcherFactory,
		instanceProviderGetter: instanceProviderGetter,
		resourceProviderGetter: resourceProviderGetter,
		modelUUID:              modelUUID,
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
	span.AddEvent(
		"resource-adoption",
		trace.StringAttr("model_uuid", s.modelUUID),
		trace.StringAttr("source_controller_version", sourceControllerVersion.String()),
	)

	provider, err := s.resourceProviderGetter(ctx)

	// Provider doesn't support adopting resources and this is ok!
	if errors.Is(err, coreerrors.NotSupported) {
		return nil
	} else if err != nil {
		err = errors.Errorf(
			"getting resource provider for adopting model cloud resources: %w",
			err,
		)
		span.RecordError(err)
		return err
	}

	controllerUUID, err := s.modelState.GetControllerUUID(ctx)
	if err != nil {
		err = errors.Errorf(
			"cannot get controller uuid while adopting model cloud resources: %w",
			err,
		)
		span.RecordError(err)
		return err
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
		err = errors.Errorf("cannot adopt cloud resources for model: %w", err)
		span.RecordError(err)
		return err
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
		err = errors.Errorf(
			"cannot get provider for model when checking for machine discrepancies in migrated model: %w",
			err,
		)
		span.RecordError(err)
		return nil, err
	}

	// If the provider doesn't support machines we can bail out early.
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, nil
	}

	providerInstances, err := provider.AllInstances(ctx)
	if err != nil {
		err = errors.Errorf(
			"cannot get all provider instances for model when checking machines: %w",
			err,
		)
		span.RecordError(err)
		return nil, err
	}

	// Build the sets of provider instance IDs and model machine instance IDs.
	providerInstanceIDsSet := make(set.Strings, len(providerInstances))
	for _, instance := range providerInstances {
		providerInstanceIDsSet.Add(instance.Id().String())
	}

	instanceIDsSet, err := s.modelState.GetAllInstanceIDs(ctx)
	if err != nil {
		err = errors.Errorf("cannot get all instance IDs for model when checking machines: %w", err)
		span.RecordError(err)
		return nil, err
	}
	// First check that all the instance IDs in the model are in the provider.
	if difference := instanceIDsSet.Difference(providerInstanceIDsSet); difference.Size() > 0 {
		err := errors.Errorf("instance IDs %q are not part of the provider instance IDs", difference.Values())
		span.RecordError(err)
		return nil, err
	}
	// Then check that all the instance ids in the provider correspond to model
	// machines instance IDs
	if difference := providerInstanceIDsSet.Difference(instanceIDsSet); difference.Size() > 0 {
		err := errors.Errorf("provider instance IDs %q are not part of the model machines instance IDs", difference.Values())
		span.RecordError(err)
		return nil, err
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
func (s *Service) InitiateMigration(ctx context.Context, targetInfo migration.TargetInfo, _ string) (string, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	span.AddEvent(
		"migration-initiation",
		trace.StringAttr("model_uuid", s.modelUUID),
		trace.StringAttr("target_controller_uuid", targetInfo.ControllerUUID),
		trace.IntAttr("target_address_count", len(targetInfo.Addrs)),
	)
	// TODO(modelmigration): implement migration info reporting.
	err := errors.ConstError("migration is not implemented")
	span.RecordError(err)
	return "", err
}

// WatchForMigration returns a notification watcher that fires when this model
// undergoes migration.
func (s *Service) WatchForMigration(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	namespace := s.modelState.GetNamespaceModelMigrating()

	w, err := s.watcherFactory.NewNotifyWatcher(
		ctx,
		"watch for model migration",
		eventsource.PredicateFilter(
			namespace,
			changestream.All,
			eventsource.EqualsPredicate(s.modelUUID),
		),
	)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return w, nil
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
	span.AddEvent(
		"minion-report.unit",
		trace.StringAttr("unit_name", unitName.String()),
		trace.StringAttr("phase", phase.String()),
	)
	// TODO(modelmigration): implement reporting phase from a unit.
	err := errors.ConstError("migration report from a unit is not implemented")
	span.RecordError(err)
	return err
}

// ReportFromMachine accepts a phase report from a migration minion for a
// machine agent.
func (s *Service) ReportFromMachine(ctx context.Context, machineName machine.Name, phase migration.Phase) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	span.AddEvent(
		"minion-report.machine",
		trace.StringAttr("machine_name", machineName.String()),
		trace.StringAttr("phase", phase.String()),
	)
	// TODO(modelmigration): implement reporting phase from a machine.
	err := errors.ConstError("migration report from a machine is not implemented")
	span.RecordError(err)
	return err
}

// SetMigrationPhase is called by the migration master to progress migration.
func (s *Service) SetMigrationPhase(ctx context.Context, phase migration.Phase) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	span.AddEvent(
		"migration-phase-set",
		trace.StringAttr("phase", phase.String()),
	)
	// TODO(modelmigration): implement reporting phase from migration master.
	err := errors.ConstError("setting migration phase is not implemented")
	span.RecordError(err)
	return err
}

// SetMigrationStatusMessage is called by the migration master to report on
// migration status.
func (s *Service) SetMigrationStatusMessage(ctx context.Context, message string) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	span.AddEvent(
		"migration-status-message",
		trace.IntAttr("message_length", len(message)),
	)
	// TODO(modelmigration): implement setting migration status message.
	err := errors.ConstError("setting migration status message is not implemented")
	span.RecordError(err)
	return err
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
	err := errors.ConstError("getting minion reports is not implemented")
	span.RecordError(err)
	return migration.MinionReports{}, err
}

// ActivateImport finalises the import of the model by clearing the
// model_migrating table entry in the model database.
func (s *Service) ActivateImport(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Before we activate the model after the import, we need to update the
	// agent version to match the current controller version. This ensures that
	// all agents after a migration are running the correct version. This was
	// done previously in two steps, and could cause a model after a migration
	// to be in a state where it was running a very old agent version until the
	// the operator manually upgraded the agents.

	desiredTargetVersionStr, err := s.controllerState.GetControllerTargetVersion(ctx)
	if err != nil {
		err = errors.Errorf("getting current controller agent version: %w", err)
		span.RecordError(err)
		return err
	} else if desiredTargetVersionStr == "" {
		// This shouldn't happen, and indicates a programming error somewhere.
		err := errors.Errorf("current controller agent version is not set")
		span.RecordError(err)
		return err
	}
	desiredTargetVersion, err := semversion.Parse(desiredTargetVersionStr)
	if err != nil {
		err = errors.Errorf(
			"parsing current controller agent version %q: %w",
			desiredTargetVersionStr,
			err,
		)
		span.RecordError(err)
		return err
	}

	currentTargetVersionStr, err := s.modelState.GetModelTargetAgentVersion(ctx)
	if err != nil {
		err = errors.Errorf("getting current model agent version: %w", err)
		span.RecordError(err)
		return err
	}
	currentTargetVersion, err := semversion.Parse(currentTargetVersionStr)
	if err != nil {
		err = errors.Errorf(
			"parsing current model agent version %q: %w",
			currentTargetVersionStr,
			err,
		)
		span.RecordError(err)
		return err
	}

	// TODO (stickupkid): We should validate if we have all the binaries
	// architectures for the desired target version here.

	// If the current target version doesn't match the desired target version,
	// we need to update it.
	if currentTargetVersion != desiredTargetVersion {
		span.AddEvent(
			"import-activation.model-target-version-update",
			trace.StringAttr("from_version", currentTargetVersion.String()),
			trace.StringAttr("to_version", desiredTargetVersion.String()),
		)
		// Update the model target agent version to match the controller's
		// target agent version.
		if err = s.modelState.SetModelTargetAgentVersion(
			ctx, currentTargetVersion.String(), desiredTargetVersion.String(),
		); err != nil {
			err = errors.Capture(err)
			span.RecordError(err)
			return err
		}
	}

	// Delete the migration importing status from the model database. This
	// should ensure that the model is no longer considered to be importing.

	// As we need to affect both the controller and model databases, we need to
	// attempt this is a best effort manner. The state layer should ensure
	// idempotency, so if one operation succeeds and the other fails, we can
	// retry safely.

	// Attempt to delete the importing status from the model database first, as
	// that should allow the model to be considered active in this controller.
	// The controller database entry can be removed later if this step fails,
	// it shouldn't prevent the model from being used (in theory).

	if err := s.modelState.DeleteModelImportingStatus(ctx); err != nil {
		err = errors.Errorf(
			"deleting model importing status from model database: %w",
			err,
		)
		span.RecordError(err)
		return err
	}
	span.AddEvent(
		"import-activation.model-import-status-cleared",
		trace.StringAttr("model_uuid", s.modelUUID),
	)

	if err := s.controllerState.DeleteModelImportingStatus(ctx, s.modelUUID); err != nil {
		err = errors.Errorf(
			"deleting model importing status from controller database: %w",
			err,
		)
		span.RecordError(err)
		return err
	}
	span.AddEvent(
		"import-activation.controller-import-status-cleared",
		trace.StringAttr("model_uuid", s.modelUUID),
	)

	return nil
}
