// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/controllernode"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/naturalsort"
	"github.com/juju/juju/internal/uuid"
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

	// NamespaceForWatchExport returns the changestream namespace for export
	// migration start/end changes keyed by model UUID.
	NamespaceForWatchExport() string

	// NamespaceForWatchPhase returns the changestream namespace for export
	// migration phase transitions keyed by model UUID.
	NamespaceForWatchPhase() string

	// NamespaceForWatchMinionSync returns the changestream namespace for minion
	// sync report changes keyed by migration UUID.
	NamespaceForWatchMinionSync() string

	// InsertExport records a new export migration attempt for a model,
	// returning [modelmigrationerrors.ErrMigrationAlreadyActive] if the model already
	// has an active export.
	InsertExport(ctx context.Context, spec modelmigrationinternal.MigrationSpec) error

	// GetActiveExport returns the active export migration for the
	// model, or [modelmigrationerrors.ErrMigrationNotFound] if none exists.
	GetActiveExport(ctx context.Context, modelUUID string) (modelmigrationinternal.Migration, error)

	// GetActiveExportUUID returns the UUID of the active export migration for
	// the model, or [modelmigrationerrors.ErrMigrationNotFound] if none exists.
	GetActiveExportUUID(ctx context.Context, modelUUID string) (string, error)

	// GetMigrationMode derives the migration mode for the model.
	GetMigrationMode(ctx context.Context, modelUUID string) (modelmigration.MigrationMode, error)

	// SetPhase transitions an export migration to a new phase, enforcing valid
	// phase transitions with optimistic locking.
	SetPhase(ctx context.Context, migrationUUID string, newPhase migration.Phase) error

	// SetStatusMessage appends a status message to an export migration.
	SetStatusMessage(ctx context.Context, migrationUUID, message string) error

	// InsertMinionReport records a phase report from a single minion agent.
	InsertMinionReport(ctx context.Context, migrationUUID string, phase migration.Phase, entityKey string, success bool) error

	// AggregateMinionReports returns the succeeded and failed entity keys
	// reported for the given migration and phase.
	AggregateMinionReports(ctx context.Context, migrationUUID string, phase migration.Phase) (modelmigrationinternal.MinionReports, error)

	// GetControllerModelInfo reads the controller-database records scoped to
	// the given migrating model in target-portable semantic form. offerUUIDs
	// are the model's hosted offer UUIDs and offererModels are the distinct
	// third-party (offerer controller, offerer model) pairs referenced by the
	// model's remote applications, both read from the model database by the
	// caller.
	GetControllerModelInfo(
		ctx context.Context,
		modelUUID string,
		offerUUIDs []string,
		offererModels []modelmigrationinternal.OffererModel,
	) (modelmigration.ControllerModelInfo, error)

	// GetSourceControllerInfo returns the source controller's identity and
	// client connection details used by the target controller to dial back
	// during model activation.
	GetSourceControllerInfo(ctx context.Context) (modelmigrationinternal.SourceControllerInfo, error)

	// CheckImportModelCollision reports model identity collisions that would
	// block importing the model on the target controller.
	CheckImportModelCollision(
		ctx context.Context, modelUUID, name, qualifier string,
	) (modelmigration.ImportModelCollision, error)

	// CheckCloudRegion reports whether the named cloud exists and, when a
	// region name is supplied, whether that region is known to the cloud.
	CheckCloudRegion(ctx context.Context, cloudName, regionName string) (
		cloudExists bool, regionExists bool, err error,
	)

	// GetDisabledUsers reports the active users from names that are disabled
	// on the controller. Missing and removed users are omitted.
	GetDisabledUsers(ctx context.Context, names []string) ([]string, error)

	// GetCredentialRevoked reports whether a cloud credential with the given
	// natural key exists on the controller and, when it does, whether it is
	// revoked.
	GetCredentialRevoked(ctx context.Context, cloud, owner, name string) (revoked bool, exists bool, err error)

	// SecretBackendExists reports whether a secret backend with the given name
	// exists on the controller.
	SecretBackendExists(ctx context.Context, name string) (bool, error)
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

	// GetMigrationAgents returns all agents that must report migration
	// minion progress for this model.
	GetMigrationAgents(ctx context.Context) (modelmigrationinternal.MigrationAgents, error)

	// GetOfferUUIDs returns the UUIDs of all offers hosted by this model, used
	// to select the offer-scoped permission rows that travel with the migration.
	GetOfferUUIDs(ctx context.Context) ([]string, error)

	// GetThirdPartyOffererModels returns the distinct (offerer controller,
	// offerer model) pairs referenced by this model's remote applications,
	// excluding pairs offered by this model's own controller, used to select
	// the third-party external controllers that travel with the migration.
	GetThirdPartyOffererModels(ctx context.Context) ([]modelmigrationinternal.OffererModel, error)
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

	provider, err := s.resourceProviderGetter(ctx)

	// Provider doesn't support adopting resources and this is ok!
	if errors.Is(err, coreerrors.NotSupported) {
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting resource provider for adopting model cloud resources: %w",
			err,
		)
	}

	controllerUUID, err := s.modelState.GetControllerUUID(ctx)
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

	instanceIDsSet, err := s.modelState.GetAllInstanceIDs(ctx)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mode, err := s.controllerState.GetMigrationMode(ctx, s.modelUUID)
	if err != nil {
		return modelmigration.MigrationModeNone, errors.Capture(err)
	}
	return mode, nil
}

// Migration returns status about migration of this model. If the model is not
// currently being migrated, a migration with phase [migration.NONE] is
// returned.
func (s *Service) Migration(ctx context.Context) (modelmigration.Migration, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mig, err := s.controllerState.GetActiveExport(ctx, s.modelUUID)
	if errors.Is(err, modelmigrationerrors.ErrMigrationNotFound) {
		return modelmigration.Migration{Phase: migration.NONE}, nil
	} else if err != nil {
		return modelmigration.Migration{}, errors.Capture(err)
	}
	return decodeMigration(mig)
}

// GetControllerModelInfo reads the controller-database records scoped to this
// migrating model and returns them in target-portable semantic form. It first
// reads the model's hosted offer UUIDs and third-party remote-offerer pairs
// from the model database, then reads the matching controller-database rows.
func (s *Service) GetControllerModelInfo(ctx context.Context) (modelmigration.ControllerModelInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	offerUUIDs, err := s.modelState.GetOfferUUIDs(ctx)
	if err != nil {
		return modelmigration.ControllerModelInfo{}, errors.Errorf("reading model offer UUIDs: %w", err)
	}
	offererModels, err := s.modelState.GetThirdPartyOffererModels(ctx)
	if err != nil {
		return modelmigration.ControllerModelInfo{}, errors.Errorf("reading model offerer models: %w", err)
	}

	info, err := s.controllerState.GetControllerModelInfo(ctx, s.modelUUID, offerUUIDs, offererModels)
	if err != nil {
		return modelmigration.ControllerModelInfo{}, errors.Errorf("reading controller model info for %q: %w", s.modelUUID, err)
	}
	return info, nil
}

// SourceControllerInfo returns this (source) controller's identity and the
// client connection details a target controller uses to dial back during model
// activation.
func (s *Service) SourceControllerInfo(ctx context.Context) (migration.SourceControllerInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	info, err := s.controllerState.GetSourceControllerInfo(ctx)
	if err != nil {
		return migration.SourceControllerInfo{}, errors.Capture(err)
	}

	// A target controller dials these addresses back to advance the migration
	// state machine and ultimately reap the source model. Without at least one
	// usable address the migration can never complete, so refuse to act as a
	// source rather than proceed into a stuck state.
	addrs := sourceControllerAddrsForClients(info.Addrs)
	if len(addrs) == 0 {
		return migration.SourceControllerInfo{}, errors.Errorf(
			"controller %q cannot be a migration source: %w",
			info.ControllerUUID, modelmigrationerrors.ErrSourceControllerNoAPIAddresses)
	}

	return migration.SourceControllerInfo{
		ControllerTag:   names.NewControllerTag(info.ControllerUUID),
		ControllerAlias: info.ControllerAlias,
		Addrs:           addrs,
		CACert:          info.CACert,
	}, nil
}

func sourceControllerAddrsForClients(addrs []modelmigrationinternal.SourceControllerAddress) []string {
	clientAddrs := sourceControllerAddrsByControllerID(addrs)
	controllerIDs := sourceControllerAddressKeyOrder(clientAddrs)

	orderedAddrs := make([]string, 0)
	for _, id := range controllerIDs {
		addrs := clientAddrs[id]
		if len(addrs) == 0 {
			continue
		}
		orderedAddrs = append(
			orderedAddrs,
			addrs.PrioritizedForScope(controllernode.ScopeMatchPublic)...,
		)
	}
	return orderedAddrs
}

func sourceControllerAddrsByControllerID(
	addrs []modelmigrationinternal.SourceControllerAddress,
) map[string]controllernode.APIAddresses {
	grouped := make(map[string]controllernode.APIAddresses)
	for _, addr := range addrs {
		grouped[addr.ControllerID] = append(grouped[addr.ControllerID], controllernode.APIAddress{
			Address: addr.Address,
			IsAgent: addr.IsAgent,
			Scope:   network.Scope(addr.Scope),
		})
	}
	return grouped
}

func sourceControllerAddressKeyOrder(m map[string]controllernode.APIAddresses) []string {
	if len(m) == 0 {
		return nil
	}

	ids := make([]string, 0, len(m))
	for controllerID := range m {
		ids = append(ids, controllerID)
	}

	sort.Strings(ids)
	return ids
}

// InitiateMigration kicks off migrating this model to the target controller,
// recording a new export migration and returning its UUID. It returns
// [modelmigrationerrors.ErrMigrationAlreadyActive] if the model is already being
// migrated.
func (s *Service) InitiateMigration(ctx context.Context, targetInfo migration.TargetInfo) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := targetInfo.Validate(); err != nil {
		return "", errors.Errorf("validating migration target: %w", err)
	}

	migrationUUID, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	macaroonsJSON, err := marshalMacaroons(targetInfo.Macaroons)
	if err != nil {
		return "", errors.Errorf("marshalling target macaroons: %w", err)
	}

	targetAddrs := make([]modelmigrationinternal.ExternalControllerAddress, len(targetInfo.Addrs))
	for i, addr := range targetInfo.Addrs {
		addrUUID, err := uuid.NewUUID()
		if err != nil {
			return "", errors.Capture(err)
		}
		targetAddrs[i] = modelmigrationinternal.ExternalControllerAddress{
			UUID:    addrUUID.String(),
			Address: addr,
		}
	}

	spec := modelmigrationinternal.MigrationSpec{
		MigrationUUID:         migrationUUID.String(),
		ModelUUID:             s.modelUUID,
		TargetControllerUUID:  targetInfo.ControllerUUID,
		TargetControllerAlias: targetInfo.ControllerAlias,
		TargetAddrs:           targetAddrs,
		TargetCACert:          targetInfo.CACert,
		TargetUser:            targetInfo.User,
		TargetMacaroons:       macaroonsJSON,
		TargetToken:           targetInfo.Token,
		TargetSkipUserChecks:  targetInfo.SkipUserChecks,
	}
	if err := s.controllerState.InsertExport(ctx, spec); err != nil {
		return "", errors.Capture(err)
	}
	return migrationUUID.String(), nil
}

func decodeMigration(mig modelmigrationinternal.Migration) (modelmigration.Migration, error) {
	macaroons, err := unmarshalMacaroons(mig.Target.Macaroons)
	if err != nil {
		return modelmigration.Migration{}, errors.Errorf("unmarshalling target macaroons: %w", err)
	}
	return modelmigration.Migration{
		UUID:             mig.UUID,
		Phase:            mig.Phase,
		PhaseChangedTime: mig.PhaseChangedTime,
		Target: migration.TargetInfo{
			ControllerUUID:  mig.Target.ControllerUUID,
			ControllerAlias: mig.Target.ControllerAlias,
			Addrs:           mig.Target.Addrs,
			CACert:          mig.Target.CACert,
			User:            mig.Target.User,
			Macaroons:       macaroons,
			Token:           mig.Target.Token,
			SkipUserChecks:  mig.Target.SkipUserChecks,
		},
	}, nil
}

// marshalMacaroons serialises a slice of macaroon slices to the JSON form
// stored in model_migration_export_target_auth.target_macaroons.
func marshalMacaroons(macaroons []macaroon.Slice) (string, error) {
	if len(macaroons) == 0 {
		return "", nil
	}
	b, err := json.Marshal(macaroons)
	if err != nil {
		return "", errors.Capture(err)
	}
	return string(b), nil
}

// unmarshalMacaroons reverses marshalMacaroons.
func unmarshalMacaroons(data string) ([]macaroon.Slice, error) {
	if data == "" {
		return nil, nil
	}
	var macaroons []macaroon.Slice
	if err := json.Unmarshal([]byte(data), &macaroons); err != nil {
		return nil, errors.Capture(err)
	}
	return macaroons, nil
}

// WatchForMigration returns a notification watcher that fires when this model
// starts or stops undergoing migration. Intermediate phase transitions are
// reported by WatchMigrationPhase.
func (s *Service) WatchForMigration(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watchControllerNamespace(
		ctx, "watch for model migration", s.controllerState.NamespaceForWatchExport(),
	)
}

// WatchMigrationPhase returns a notification watcher that fires on each of this
// model's migration phase transitions.
func (s *Service) WatchMigrationPhase(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watchControllerNamespace(
		ctx, "watch for migration phase change", s.controllerState.NamespaceForWatchPhase(),
	)
}

func (s *Service) watchControllerNamespace(
	ctx context.Context, summary, namespace string,
) (watcher.NotifyWatcher, error) {
	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		summary,
		eventsource.PredicateFilter(
			namespace,
			changestream.All,
			eventsource.EqualsPredicate(s.modelUUID),
		),
	)
}

// ReportMinion accepts a phase report from a migration minion agent.
func (s *Service) ReportMinion(ctx context.Context, entityKey string, phase migration.Phase, success bool) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mig, err := s.controllerState.GetActiveExport(ctx, s.modelUUID)
	if err != nil {
		return errors.Capture(err)
	}
	return s.controllerState.InsertMinionReport(ctx, mig.UUID, phase, entityKey, success)
}

// SetMigrationPhase is called by the migration master to progress migration.
func (s *Service) SetMigrationPhase(ctx context.Context, phase migration.Phase) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mig, err := s.controllerState.GetActiveExport(ctx, s.modelUUID)
	if err != nil {
		return errors.Capture(err)
	}
	return s.controllerState.SetPhase(ctx, mig.UUID, phase)
}

// MarkModelAsGone is called by the migration master during REAP, once the
// target controller owns the model, to remove the migrated model from this
// controller. It marks the active export migration as DONE.
//
// TODO(modelmigration): purge the migrated model from the source controller
// and set up the durable login redirect before completing the export.
func (s *Service) MarkModelAsGone(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mig, err := s.controllerState.GetActiveExport(ctx, s.modelUUID)
	if err != nil {
		return errors.Capture(err)
	}
	return s.controllerState.SetPhase(ctx, mig.UUID, migration.DONE)
}

// SetMigrationStatusMessage is called by the migration master to report on
// migration status.
func (s *Service) SetMigrationStatusMessage(ctx context.Context, message string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mig, err := s.controllerState.GetActiveExport(ctx, s.modelUUID)
	if err != nil {
		return errors.Capture(err)
	}
	return s.controllerState.SetStatusMessage(ctx, mig.UUID, message)
}

// WatchMinionReports returns a notification watcher that fires when any minion
// reports an update to their phase for this model's active migration.
func (s *Service) WatchMinionReports(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	migUUID, err := s.controllerState.GetActiveExportUUID(ctx, s.modelUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"watch for migration minion reports",
		eventsource.PredicateFilter(
			s.controllerState.NamespaceForWatchMinionSync(),
			changestream.All,
			eventsource.EqualsPredicate(migUUID),
		),
	)
}

// MinionReports returns phase information about minions in this model for the
// active migration's current phase.
func (s *Service) MinionReports(ctx context.Context) (migration.MinionReports, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mig, err := s.controllerState.GetActiveExport(ctx, s.modelUUID)
	if err != nil {
		return migration.MinionReports{}, errors.Capture(err)
	}

	aggregated, err := s.controllerState.AggregateMinionReports(ctx, mig.UUID, mig.Phase)
	if err != nil {
		return migration.MinionReports{}, errors.Capture(err)
	}

	migrationAgents, err := s.modelState.GetMigrationAgents(ctx)
	if err != nil {
		return migration.MinionReports{}, errors.Capture(err)
	}
	allAgents := migrationAgentKeys(migrationAgents)

	succeeded := set.NewStrings(aggregated.Succeeded...)
	failed := set.NewStrings(aggregated.Failed...)
	unknown := allAgents.Difference(succeeded).Difference(failed)

	reports := migration.MinionReports{
		MigrationId:  mig.UUID,
		Phase:        mig.Phase,
		TotalCount:   allAgents.Size(),
		SuccessCount: succeeded.Size(),
		UnknownCount: unknown.Size(),
	}
	for _, key := range naturalsort.Sort(failed.Values()) {
		if err := addMinionReportEntity(
			key,
			&reports.FailedMachines,
			&reports.FailedUnits,
			&reports.FailedApplications,
		); err != nil {
			return migration.MinionReports{}, errors.Capture(err)
		}
	}
	for _, key := range naturalsort.Sort(unknown.Values()) {
		if len(reports.SomeUnknownMachines)+
			len(reports.SomeUnknownUnits)+
			len(reports.SomeUnknownApplications) >= 10 {
			break
		}
		if err := addMinionReportEntity(
			key,
			&reports.SomeUnknownMachines,
			&reports.SomeUnknownUnits,
			&reports.SomeUnknownApplications,
		); err != nil {
			return migration.MinionReports{}, errors.Capture(err)
		}
	}
	return reports, nil
}

func migrationAgentKeys(agents modelmigrationinternal.MigrationAgents) set.Strings {
	result := set.NewStrings()
	for _, machineName := range agents.Machines {
		result.Add(machineMinionReportKey(machineName))
	}
	for _, unitName := range agents.Units {
		result.Add(unitMinionReportKey(unitName))
	}
	for _, applicationName := range agents.Applications {
		result.Add(applicationMinionReportKey(applicationName))
	}
	return result
}

const (
	machineMinionReportKeyPrefix     = "machine-"
	unitMinionReportKeyPrefix        = "unit-"
	applicationMinionReportKeyPrefix = "application-"
	minionReportKeySeparator         = "-"
)

func machineMinionReportKey(name string) string {
	return machineMinionReportKeyPrefix + strings.ReplaceAll(name, "/", minionReportKeySeparator)
}

func unitMinionReportKey(name string) string {
	return unitMinionReportKeyPrefix + strings.ReplaceAll(name, "/", minionReportKeySeparator)
}

func applicationMinionReportKey(name string) string {
	return applicationMinionReportKeyPrefix + name
}

func addMinionReportEntity(
	key string,
	machines *[]string,
	units *[]string,
	applications *[]string,
) error {
	if name, ok := strings.CutPrefix(key, machineMinionReportKeyPrefix); ok {
		*machines = append(*machines, machineNameFromMinionReportKey(name))
		return nil
	}
	if name, ok := strings.CutPrefix(key, unitMinionReportKeyPrefix); ok {
		unitName, err := unitNameFromMinionReportKey(name)
		if err != nil {
			return errors.Errorf("parsing reported entity %q: %w", key, err)
		}
		*units = append(*units, unitName)
		return nil
	}
	if name, ok := strings.CutPrefix(key, applicationMinionReportKeyPrefix); ok && name != "" {
		*applications = append(*applications, name)
		return nil
	}
	return errors.Errorf("unsupported migration minion entity key %q", key)
}

func machineNameFromMinionReportKey(key string) string {
	parts := strings.Split(key, minionReportKeySeparator)
	if len(parts) == 1 {
		return key
	}
	return parts[0] + "/" + strings.Join(parts[1:], "/")
}

func unitNameFromMinionReportKey(key string) (string, error) {
	appName, unitNumber, ok := strings.Cut(key, minionReportKeySeparator)
	for ok {
		nextAppName, nextUnitNumber, nextOk := strings.Cut(unitNumber, minionReportKeySeparator)
		if !nextOk {
			return appName + "/" + unitNumber, nil
		}
		appName += minionReportKeySeparator + nextAppName
		unitNumber = nextUnitNumber
	}
	return "", errors.Errorf("missing unit number")
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
		return errors.Errorf("getting current controller agent version: %w", err)
	} else if desiredTargetVersionStr == "" {
		// This shouldn't happen, and indicates a programming error somewhere.
		return errors.Errorf("current controller agent version is not set")
	}

	desiredTargetVersion, err := semversion.Parse(desiredTargetVersionStr)
	if err != nil {
		return errors.Errorf(
			"parsing current controller agent version %q: %w",
			desiredTargetVersionStr,
			err,
		)
	}

	currentTargetVersionStr, err := s.modelState.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return errors.Errorf("getting current model agent version: %w", err)
	}

	currentTargetVersion, err := semversion.Parse(currentTargetVersionStr)
	if err != nil {
		return errors.Errorf(
			"parsing current model agent version %q: %w",
			currentTargetVersionStr,
			err,
		)
	}

	// TODO (stickupkid): We should validate if we have all the binaries
	// architectures for the desired target version here.

	// If the current target version doesn't match the desired target version,
	// we need to update it.
	if currentTargetVersion != desiredTargetVersion {
		// Update the model target agent version to match the controller's
		// target agent version.
		if err = s.modelState.SetModelTargetAgentVersion(
			ctx, currentTargetVersion.String(), desiredTargetVersion.String(),
		); err != nil {
			return errors.Capture(err)
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
		return errors.Errorf(
			"deleting model importing status from model database: %w",
			err,
		)
	}

	if err := s.controllerState.DeleteModelImportingStatus(ctx, s.modelUUID); err != nil {
		return errors.Errorf(
			"deleting model importing status from controller database: %w",
			err,
		)
	}

	return nil
}
