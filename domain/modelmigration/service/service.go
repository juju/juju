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
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/migration"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
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
	logger          logger.Logger
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

	// GetConflictingCloudImageMetadata reports, for each supplied custom image
	// metadata row, the existing target image id when a row with the same
	// natural key already exists on the controller with a different image id.
	GetConflictingCloudImageMetadata(ctx context.Context, rows []modelmigration.ImportPrecheckImageMetadata) ([]modelmigration.CloudImageMetadataConflict, error)

	// BeginImport inserts a new durable model_migration_import claim
	// (phase=importing) for modelUUID as the first target-side write of a v8
	// import, using claimUUID as the claim's UUID, and returns the resulting
	// claim. If a claim already exists, the existing claim is returned
	// alongside [modelmigrationerrors.ErrImportClaimExists].
	BeginImport(ctx context.Context, modelUUID, claimUUID, sourceMigrationUUID string) (modelmigration.ImportClaim, error)

	// GetImportClaim returns the target-side import claim for the given model
	// UUID, or [modelmigrationerrors.ErrImportNotFound] when no claim exists.
	GetImportClaim(ctx context.Context, modelUUID string) (modelmigration.ImportClaim, error)

	// AssertImporting returns nil if a model_migration_import claim exists for
	// modelUUID and its phase is 'importing'. It returns
	// [modelmigrationerrors.ErrImportNotFound] if no claim exists, or
	// [modelmigrationerrors.ErrImportNotImporting] if the claim has moved past
	// the importing phase.
	AssertImporting(ctx context.Context, modelUUID string) error

	// ImportOfferPermissions records the offer UUIDs granted permission during
	// this import claim into model_migration_import_offer, atomically with an
	// importing-phase assertion for modelUUID.
	ImportOfferPermissions(ctx context.Context, modelUUID, claimUUID string, offerUUIDs []string) error

	// EnsureExternalControllerExists compares-or-inserts a single third-party
	// controller's connection details, failing with
	// [modelmigrationerrors.ErrExternalControllerMismatch] on a mismatch
	// rather than overwriting live CMR connection data.
	EnsureExternalControllerExists(ctx context.Context, ref modelmigrationinternal.ExternalController) error

	// ImportExternalControllers applies the third-party external controller
	// references from a v8 import envelope to the target controller,
	// atomically with an importing-phase assertion for modelUUID, and records
	// the durable (offerer_model_uuid, controller_uuid) handoff for Activate.
	ImportExternalControllers(
		ctx context.Context, modelUUID, claimUUID string, refs []modelmigrationinternal.ExternalController,
	) error

	// GetImportedOfferUUIDs returns the offer UUIDs recorded in
	// model_migration_import_offer for the import claim of the given model.
	// Returns nil (not an error) when no offer rows exist.
	GetImportedOfferUUIDs(ctx context.Context, modelUUID string) ([]string, error)

	// SetImportPhaseActivating transitions the model's import claim from the
	// importing phase to the activating phase. It is idempotent when the
	// claim is already activating and returns
	// [modelmigrationerrors.ErrActivationAborting] when the claim is aborting.
	SetImportPhaseActivating(ctx context.Context, modelUUID string) error

	// DeleteActivatedImport removes the model's import claim and its
	// FK-dependent companion rows, asserting the claim is in the activating
	// phase. It is idempotent when no claim exists.
	DeleteActivatedImport(ctx context.Context, modelUUID string) error

	// EnsureSourceControllerExists compares-or-inserts the migration source
	// controller's connection details and records the models it offers,
	// failing with [modelmigrationerrors.ErrExternalControllerMismatch] on a
	// mismatch rather than overwriting live CMR connection data.
	EnsureSourceControllerExists(
		ctx context.Context, controllerUUID, alias, caCert string, addrs, addrUUIDs, consumedModels []string,
	) error

	// ExternalControllerModelsForImport returns the third-party offerer-model
	// to controller mappings recorded for the model's import claim. Returns an
	// empty slice when no mappings exist or the model has no claim.
	ExternalControllerModelsForImport(ctx context.Context, modelUUID string) ([]coremodelmigration.OffererModel, error)

	// GetControllerTargetVersion returns the controller's target agent version.
	GetControllerTargetVersion(ctx context.Context) (string, error)

	// EnsureExportOffers records the hosted offer UUIDs for a migration into
	// model_migration_export_offer. Idempotent.
	EnsureExportOffers(ctx context.Context, migrationUUID string, offerUUIDs []string) error

	// StageModelRedirect writes the redirect snapshot with completed_at = NULL.
	// Idempotent.
	StageModelRedirect(
		ctx context.Context,
		migrationUUID, modelUUID string,
		target modelmigrationinternal.RedirectionTarget,
		users []modelmigrationinternal.RedirectUserAccess,
	) error

	// GetModelUsersForRedirect returns the model-scoped permission rows
	// joined with user identity, used to populate the redirect user snapshot.
	GetModelUsersForRedirect(ctx context.Context, modelUUID string) ([]modelmigrationinternal.RedirectUserAccess, error)

	// CompleteModelRedirectAndPurge runs the final controller-DB transaction
	// of source REAP: purges model-scoped rows, stages the model database
	// deletion, completes the redirect, marks the export DONE, and scrubs
	// target auth. It fails unless the export is still in phase REAP.
	CompleteModelRedirectAndPurge(ctx context.Context, migrationUUID, modelUUID string) error
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

	// GetMigrationAgents returns all agents that must report migration
	// minion progress for this model.
	GetMigrationAgents(ctx context.Context) (modelmigrationinternal.MigrationAgents, error)

	// GetOfferUUIDs returns the UUIDs of all offers hosted by this model, used
	// to select the offer-scoped permission rows that travel with the migration.
	GetOfferUUIDs(ctx context.Context) ([]string, error)

	// DeleteModelImportingStatus clears the model-database import gate, making
	// the model visible once activation completes.
	DeleteModelImportingStatus(ctx context.Context) error

	// GetModelTargetAgentVersion returns the target agent version currently set
	// for the model.
	GetModelTargetAgentVersion(ctx context.Context) (string, error)

	// SetModelTargetAgentVersion sets the model's target agent version,
	// asserting that the current version matches preCondition.
	SetModelTargetAgentVersion(ctx context.Context, preCondition, toVersion string) error
}

// NewImportService constructs a new [Service] for the v8 import driver, which
// only needs controller-scoped claim methods. The model-export-only
// dependencies (modelState, watcherFactory, the provider getters, modelUUID)
// are intentionally left unset rather than passed as nil by the caller.
func NewImportService(controllerState ControllerState, logger logger.Logger) *Service {
	return &Service{
		controllerState: controllerState,
		logger:          logger,
	}
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
	logger logger.Logger,
) *Service {
	return &Service{
		controllerState:        controllerState,
		modelState:             modelState,
		watcherFactory:         watcherFactory,
		instanceProviderGetter: instanceProviderGetter,
		resourceProviderGetter: resourceProviderGetter,
		modelUUID:              modelUUID,
		logger:                 logger,
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
// controller. It runs the following steps in order:
//
//  1. Read the active export; if there is no active export it is already DONE
//     from a previous run and this is a no-op.
//  2. Capture the hosted offer UUIDs from the model DB.
//  3. Stage the redirect snapshot (completed_at = NULL, not yet active).
//  4. Run the controller-DB purge transaction: delete model-scoped rows, stage
//     the model database deletion, complete the redirect, mark the export DONE,
//     and scrub target auth.
//
// The purge transaction in step 4 is the single commit point. Everything
// before it is an idempotent preparation that leaves the model fully intact,
// so a failure or crash before step 4 commits can simply be retried. Once it
// commits, the model is gone from the controller database and the redirect is
// active. The model's dqlite database is not deleted here: step 4 stages the
// deletion, and the model DB deleter worker deletes the database
// asynchronously, retrying until the staged row is gone.
//
// It never calls normal model removal, removal jobs, undertaker provider
// deletion, or provider Destroy — it only purges rows belonging to a model
// that already lives on another controller.
func (s *Service) MarkModelAsGone(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mig, err := s.controllerState.GetActiveExport(ctx, s.modelUUID)
	if err != nil {
		if errors.Is(err, modelmigrationerrors.ErrMigrationNotFound) {
			// No active export — already DONE from a previous run. Idempotent.
			return nil
		}
		return errors.Capture(err)
	}
	if mig.Phase != migration.REAP {
		return errors.Errorf(
			"cannot reap migration %q in phase %q: expected %q: %w",
			mig.UUID, mig.Phase, migration.REAP, modelmigrationerrors.ErrPhaseTransitionInvalid,
		)
	}

	// Step 2: Capture hosted offer UUIDs from the model DB, so the purge can
	// delete their permission rows without the model DB. The model DB is
	// still present on every retry because it is only deleted after the
	// purge transaction commits, at which point the export is DONE and this
	// method returns early above.
	offerUUIDs, err := s.modelState.GetOfferUUIDs(ctx)
	if err != nil {
		return errors.Errorf("reading hosted offer UUIDs for model %q: %w", s.modelUUID, err)
	}
	if err := s.controllerState.EnsureExportOffers(ctx, mig.UUID, offerUUIDs); err != nil {
		return errors.Errorf("capturing export offers for migration %q: %w", mig.UUID, err)
	}

	// Step 3: Stage the redirect snapshot (users + target info). Staged but
	// inactive until the purge transaction sets completed_at.
	users, err := s.controllerState.GetModelUsersForRedirect(ctx, s.modelUUID)
	if err != nil {
		return errors.Errorf("reading model users for redirect: %w", err)
	}
	target := modelmigrationinternal.RedirectionTarget{
		ControllerUUID:  mig.Target.ControllerUUID,
		ControllerAlias: mig.Target.ControllerAlias,
		Addresses:       mig.Target.Addrs,
		CACert:          mig.Target.CACert,
	}
	if err := s.controllerState.StageModelRedirect(ctx, mig.UUID, s.modelUUID, target, users); err != nil {
		return errors.Errorf("staging redirect for model %q: %w", s.modelUUID, err)
	}

	// Step 4: The controller-DB purge transaction — the commit point. On
	// success the model is gone from the controller database, the redirect
	// is active, the model database deletion is staged, and the export is DONE.
	if err := s.controllerState.CompleteModelRedirectAndPurge(ctx, mig.UUID, s.modelUUID); err != nil {
		return errors.Errorf("purging source model %q: %w", s.modelUUID, err)
	}

	return nil
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
