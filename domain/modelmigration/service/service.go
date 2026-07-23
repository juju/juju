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

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
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

// CredentialValidator checks whether the imported model's credential can
// access its cloud on this controller.
type CredentialValidator interface {
	// Validate opens the model's cloud with the given credential and reports
	// whether it grants access to the model's resources. info describes the
	// model the credential belongs to. A non-nil error means the imported model
	// must not be activated.
	Validate(
		ctx context.Context,
		info modelmigration.CredentialValidationInfo,
		credential coremodelmigration.ModelCloudCredential,
	) error
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

	controllerState     ControllerState
	modelState          ModelState
	watcherFactory      WatcherFactory
	credentialValidator CredentialValidator
	modelUUID           string
	logger              logger.Logger
}

// WatcherFactory describes methods for creating watchers used by the
// [Service].
type WatcherFactory interface {
	// NewNamespaceWatcher returns a watcher that emits the initial collection
	// members followed by changed identifiers in the input namespace.
	NewNamespaceWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

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
	// InitialWatchImportClaimsStatement returns the changestream namespace and
	// initial query for target-side import claims, keyed by model UUID.
	InitialWatchImportClaimsStatement() (string, string)

	// GetKnownSecretBackends returns the subset of the supplied secret backend
	// UUIDs that exist on the controller, used to detect model secret value
	// refs that still carry a source-controller-local backend UUID after
	// import.
	GetKnownSecretBackends(ctx context.Context, uuids []string) ([]string, error)

	// GetSecretBackendReferencesForModel returns a map from secret revision
	// UUID to the secret backend UUID recorded for it in
	// secret_backend_reference for the given model.
	GetSecretBackendReferencesForModel(ctx context.Context, modelUUID string) (map[string]string, error)

	// GetModelCloudCredential returns the natural key, auth material and
	// status of the credential assigned to the given model, or nil when the
	// model has no credential.
	GetModelCloudCredential(ctx context.Context, modelUUID string) (*coremodelmigration.ModelCloudCredential, error)

	// GetAgentBinaryArchitecturesForVersion returns the architecture names for
	// which the controller's object store holds agent binaries at the given
	// version.
	GetAgentBinaryArchitecturesForVersion(ctx context.Context, version string) ([]string, error)

	// GetCloud returns the full definition of the named cloud (auth types,
	// regions, endpoints and CA certificates).
	GetCloud(ctx context.Context, name string) (cloud.Cloud, error)

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

	// NamespaceForWatchImportClaim returns the changestream namespace for
	// target-side import claim changes keyed by model UUID.
	NamespaceForWatchImportClaim() string

	// NamespaceForWatchModelDatabaseDeletion returns the changestream namespace
	// for staged model-database deletion changes, keyed by the model's namespace
	// (its UUID).
	NamespaceForWatchModelDatabaseDeletion() string

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

	// GetMigrationPhase derives the model's migration phase in both
	// directions: importing while a target import claim exists, otherwise the
	// active export's phase, otherwise none. The phase is returned as its
	// name.
	GetMigrationPhase(ctx context.Context, modelUUID string) (string, error)

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

	// SetImportPhaseAborting transitions the model's import claim from the
	// importing phase to the aborting phase. It is idempotent when the claim is
	// already aborting and returns
	// [modelmigrationerrors.ErrAbortActivating] when the claim is activating.
	SetImportPhaseAborting(ctx context.Context, modelUUID string) error

	// FinalizeAbortedImport deletes the model's import claim, its FK-dependent
	// companion rows, and its namespace registration once abort cleanup is
	// provably complete, asserting the claim is aborting and the controller
	// model identity and namespace mapping are both gone. It returns
	// [modelmigrationerrors.ErrAbortNotFinalizable] when cleanup is not yet
	// provable, and is idempotent when no claim exists.
	FinalizeAbortedImport(ctx context.Context, modelUUID string) error

	// IsModelDying reports whether the model row exists and has left the alive
	// state (dying or dead), indicating the generic removal undertaker has taken
	// over teardown after a v7/legacy abort. A missing model row reports false.
	IsModelDying(ctx context.Context, modelUUID string) (bool, error)

	// GetAllImportClaims returns a snapshot of every outstanding
	// model_migration_import claim, used by the abort reconciler.
	GetAllImportClaims(ctx context.Context) ([]modelmigration.ImportClaimStatus, error)

	// IsImportNamespaceRegistered reports whether the model's dqlite namespace
	// is still registered, i.e. whether the model database may still need
	// dropping before abort finalization.
	IsImportNamespaceRegistered(ctx context.Context, modelUUID string) (bool, error)

	// StageAbortedModelDatabaseDeletion removes the aborted model's namespace
	// registration and stages its dqlite database for deletion by the
	// undertaker's model-database deleter. It asserts the claim is aborting and
	// is idempotent.
	StageAbortedModelDatabaseDeletion(ctx context.Context, modelUUID string) error
}

// ModelState defines the interface required for accessing the underlying state
// of the model during migration.
type ModelState interface {
	// GetControllerUUID returns the UUID of the controller that owns this
	// model.
	GetControllerUUID(context.Context) (string, error)

	// IsModelImporting reports whether the model database still carries its
	// import gate.
	IsModelImporting(ctx context.Context) (bool, error)
	// GetMachineInstanceIDs returns a map from provider cloud instance ID to the
	// name of the model machine it backs, for every provisioned machine the
	// cloud is expected to know about.
	GetMachineInstanceIDs(ctx context.Context) (map[string]string, error)
	// GetModelType returns the model's deployment type (for example "iaas" or
	// "caas").
	GetModelType(ctx context.Context) (string, error)
	// GetCredentialValidationInfo returns the model's owning controller,
	// deployment type, cloud placement and stored configuration, as needed to
	// validate the model's cloud credential.
	GetCredentialValidationInfo(ctx context.Context) (modelmigration.CredentialValidationInfo, error)
	// GetMachineInstanceID returns the provider instance ID of the machine
	// with the given UUID.
	GetMachineInstanceID(ctx context.Context, machineUUID string) (string, error)
	// GetSecretBackendUUIDsInUse returns the distinct secret backend UUIDs
	// referenced by the model's external secret value refs, including deleted
	// value refs pending cleanup.
	GetSecretBackendUUIDsInUse(ctx context.Context) ([]string, error)
	// GetExternalSecretRevisionBackends returns a map from secret revision UUID
	// to the backend UUID its external value ref points at, for revisions whose
	// content is stored externally.
	GetExternalSecretRevisionBackends(ctx context.Context) (map[string]string, error)
	// GetRelationValidationData returns the relation identities, keys and
	// endpoints used to validate imported relation-unit consistency. Only alive
	// relations are returned.
	GetRelationValidationData(ctx context.Context) ([]modelmigrationinternal.RelationValidationData, error)
	// GetSubordinateUnitPrincipals returns a map from subordinate unit name to
	// the name of the application its principal unit belongs to. Units absent
	// from the map are principals themselves.
	GetSubordinateUnitPrincipals(ctx context.Context) (map[string]string, error)
	// GetApplicationUnitNames returns a map from application name to the names
	// of its units. Only alive applications and units are returned.
	GetApplicationUnitNames(ctx context.Context) (map[string][]string, error)
	// GetRelationUnitsByApplication returns a map from relation UUID to the
	// unit names in scope for that relation, grouped by application name.
	GetRelationUnitsByApplication(ctx context.Context) (map[string]map[string][]string, error)
	// GetRunningAgentArchitectures returns the distinct architecture names
	// reported by the model's machine and unit agents.
	GetRunningAgentArchitectures(ctx context.Context) ([]string, error)
	// GetAgentBinaryArchitecturesForVersion returns the architecture names for
	// which the model's object store holds agent binaries at the given version.
	GetAgentBinaryArchitecturesForVersion(ctx context.Context, version string) ([]string, error)

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
	credentialValidator CredentialValidator,
	logger logger.Logger,
) *Service {
	return &Service{
		controllerState:        controllerState,
		modelState:             modelState,
		watcherFactory:         watcherFactory,
		instanceProviderGetter: instanceProviderGetter,
		resourceProviderGetter: resourceProviderGetter,
		credentialValidator:    credentialValidator,
		modelUUID:              modelUUID,
		logger:                 logger,
	}
}

// WatchableService provides the means for supporting model migration actions
// between controllers and the ability to create watchers.
type WatchableService struct {
	Service
}

// NewWatchableService is responsible for constructing a new
// [WatchableService] to handle model migration tasks with watching
// capabilities.
func NewWatchableService(
	controllerState ControllerState,
	modelState ModelState,
	modelUUID string,
	watcherFactory WatcherFactory,
	instanceProviderGetter providertracker.ProviderGetter[InstanceProvider],
	resourceProviderGetter providertracker.ProviderGetter[ResourceProvider],
	credentialValidator CredentialValidator,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: *NewService(
			controllerState,
			modelState,
			modelUUID,
			watcherFactory,
			instanceProviderGetter,
			resourceProviderGetter,
			credentialValidator,
			logger,
		),
	}
}

// NewWatchableImportService constructs a controller-scoped import
// [WatchableService] with the watchers the migration reconciler needs. It only
// wires the controller-scoped claim methods and the watcher factory; the
// regular import path needs no changestream dependency and continues to use
// [NewImportService].
func NewWatchableImportService(
	controllerState ControllerState,
	watcherFactory WatcherFactory,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			controllerState: controllerState,
			watcherFactory:  watcherFactory,
			logger:          logger,
		},
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
// into this target controller. We validate the model's cloud credential and
// check the machines that exist in the model against the machines reported by
// the models cloud and report any discrepancies.
//
// This is the counterpart of 3.6's migrationtarget CheckMachines facade, which
// validated the credential and reconciled the machines in the same call.
func (s *Service) CheckMachines(
	ctx context.Context,
) ([]modelmigration.MigrationMachineDiscrepancy, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	info, err := s.modelState.GetCredentialValidationInfo(ctx)
	if err != nil {
		return nil, errors.Errorf("getting credential validation info for model: %w", err)
	}

	if err := s.checkModelCredential(ctx, info); err != nil {
		return nil, errors.Errorf("validating model credential: %w", err)
	}

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

	// instanceToMachine maps each provisioned model machine's cloud instance ID
	// to its machine name, so discrepancies can name the offending machine.
	instanceToMachine, err := s.modelState.GetMachineInstanceIDs(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get instance IDs for model when checking machines: %w", err)
	}
	modelInstanceIDsSet := make(set.Strings, len(instanceToMachine))
	for instanceID := range instanceToMachine {
		modelInstanceIDsSet.Add(instanceID)
	}

	var discrepancies []modelmigration.MigrationMachineDiscrepancy

	// A model machine whose cloud instance is not reported by the provider: the
	// instance the model references does not exist in the cloud. Both fields are
	// populated.
	for _, instanceID := range modelInstanceIDsSet.Difference(providerInstanceIDsSet).SortedValues() {
		discrepancies = append(discrepancies, modelmigration.MigrationMachineDiscrepancy{
			MachineName:     machine.Name(instanceToMachine[instanceID]),
			CloudInstanceId: instance.Id(instanceID),
		})
	}

	// A provider instance not tracked by any model machine: the cloud has an
	// instance Juju does not know about. MachineName is left empty. On a cloud
	// whose machines Juju does not provision this is normal rather than a
	// discrepancy, see [checkCloudInstances].
	if checkCloudInstances(info.CloudType) {
		for _, instanceID := range providerInstanceIDsSet.Difference(modelInstanceIDsSet).SortedValues() {
			discrepancies = append(discrepancies, modelmigration.MigrationMachineDiscrepancy{
				CloudInstanceId: instance.Id(instanceID),
			})
		}
	}

	return discrepancies, nil
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

// MigrationPhase returns the migration phase of this model considering
// migration in *both* directions:
//
//   - a live target-side import claim, in any phase, reports [migration.IMPORT];
//   - otherwise an active source-side export reports its own phase;
//   - otherwise [migration.NONE].
//
// It exists because [Service.Migration] deliberately reports only exports: the
// migration master needs export identity and target info, and must never treat
// an import claim as something it should drive. Callers that only want to know
// "is this model migrating, either way?" - the migration flag on both the
// controller and agent sides - must use this instead, so that a target model
// stays frozen for the whole of an import rather than only during an export.
//
// [migration.IMPORT] is reported for every claim phase, including aborting,
// because none of them make the model usable. Since IMPORT is non-terminal, a
// flag built on [IsTerminal] reports false and workers stay parked until the
// claim is gone.
func (s *Service) MigrationPhase(ctx context.Context) (migration.Phase, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// GetMigrationPhase resolves both tables in one transaction, so it cannot
	// report a stale mix of the two sides.
	phaseName, err := s.controllerState.GetMigrationPhase(ctx, s.modelUUID)
	if err != nil {
		return migration.UNKNOWN, errors.Capture(err)
	}
	phase, ok := migration.ParsePhase(phaseName)
	if !ok {
		return migration.UNKNOWN, errors.Errorf("unknown migration phase %q", phaseName)
	}
	return phase, nil
}

// Migration returns status about migration of this model. If the model is not
// currently being migrated, a migration with phase [migration.NONE] is
// returned.
//
// This reports source-side exports only. See [Service.MigrationPhase] for the
// direction-agnostic read used by the migration flag.
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

// WatchMigrationActivity returns a notification watcher that fires whenever
// this model's migration activity changes in either direction: an export phase
// transition on the source side, or an import claim being created, changing
// phase, or being deleted on the target side.
//
// It is the watcher behind [Service.MigrationPhase], and exists because
// [Service.WatchMigrationPhase] observes exports only: a target import is
// invisible to that watcher, so on its own it would never fire when an
// imported model becomes usable. Claim deletion is exactly that moment, so
// watching both namespaces is what lets the migration flag unfreeze a target
// model. Extending [Service.WatchMigrationPhase] to also observe claims was
// rejected: it feeds the migration minion, which follows the source's phase
// machine and must not be woken by target-side claim changes.
func (s *WatchableService) WatchMigrationActivity(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"watch for migration activity",
		eventsource.PredicateFilter(
			s.controllerState.NamespaceForWatchPhase(),
			changestream.All,
			eventsource.EqualsPredicate(s.modelUUID),
		),
		eventsource.PredicateFilter(
			s.controllerState.NamespaceForWatchImportClaim(),
			changestream.All,
			eventsource.EqualsPredicate(s.modelUUID),
		),
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
