// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/agentbinary"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudimagemetadataservice "github.com/juju/juju/domain/cloudimagemetadata/service"
	cloudimagemetadatastate "github.com/juju/juju/domain/cloudimagemetadata/state"
	credentialservice "github.com/juju/juju/domain/credential/service"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/export/types/latest"
	keymanagerservice "github.com/juju/juju/domain/keymanager/service"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	leaseservice "github.com/juju/juju/domain/lease/service"
	leasestate "github.com/juju/juju/domain/lease/state"
	domainmodel "github.com/juju/juju/domain/model"
	modelservice "github.com/juju/juju/domain/model/service"
	modelmigrationservice "github.com/juju/juju/domain/model/service/migration"
	modelstatecontroller "github.com/juju/juju/domain/model/state/controller"
	modelstatemodel "github.com/juju/juju/domain/model/state/model"
	migrationclaimservice "github.com/juju/juju/domain/modelmigration/service"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	"github.com/juju/juju/internal/errors"
)

// Deps bundles the database and ambient dependencies the v8 import
// orchestrator needs, supplied by the caller's migration scope.
type Deps struct {
	ControllerDB database.TxnRunnerFactory
	ModelDB      database.TxnRunnerFactory
	Clock        clock.Clock
	Logger       logger.Logger
}

// ImportModelArgs contains the data needed to perform a v8 model import: the
// target-portable controller-scoped information and the transformed model-DB
// payload.
type ImportModelArgs struct {
	// SourceMigrationUUID is the source-side migration UUID recorded on the
	// target import claim.
	SourceMigrationUUID string

	// ControllerModelInfo is the semantic controller-database information for
	// the model, decoded from the v8 import envelope by the apiserver facade.
	ControllerModelInfo coremodelmigration.ControllerModelInfo

	// ModelDBPayload is the model-DB export payload decoded from the envelope
	// and transformed up to the target schema version by the apiserver facade.
	// The controller-scoped import in this package does not use it; the model-DB
	// import operations driven by the migration package's coordinator consume
	// it. It is nil only for controller-scoped-only callers and tests.
	ModelDBPayload *latest.ModelExport
}

// ImportControllerModelInfo applies the v8 import's controller-scoped semantic
// data to the target controller: the durable model_migration_import claim, the
// target-local model bootstrap (controller model row + model DB in importing
// mode), and the users, credential, permissions, authorized keys, secret
// backend, leadership and cloud image metadata carried by info. It writes only
// controller-database state; the model-DB content import and activation are
// separate concerns handled outside this package.
//
// Each step calls the owning domain's service import method directly. The
// coordinator constructs the controller-scoped domain services once and
// orchestrates the call order FK-/dependency-safely.
//
// It writes only controller-database state. Any source→target rewrite of the
// model-DB payload (e.g. secret backend UUIDs) is read back and applied
// separately by reconcileSecretBackendUUIDs once these writes have landed.
//
// sourceMigrationUUID is the source-side migration UUID recorded on the target
// import claim. If a claim already exists for info.ModelInfo.UUID, the returned
// error wraps [coreerrors.AlreadyExists] (phase-specific wording is supplied by
// the modelmigration domain).
func ImportControllerModelInfo(
	ctx context.Context,
	svc ImportServices,
	deps Deps,
	sourceMigrationUUID string,
	info coremodelmigration.ControllerModelInfo,
	view export.ProjectionView,
) error {
	return newImportCoordinator(svc, deps, sourceMigrationUUID, info, view).Import(ctx)
}

// RemoveOnAbortImport is the abort seam Task 11 will call from AbortImport. It
// undoes the controller-DB writes performed by ImportControllerModelInfo in
// reverse order. Each step is idempotent: it is safe to call RemoveOnAbortImport
// more than once.
func RemoveOnAbortImport(
	ctx context.Context,
	svc ImportServices,
	deps Deps,
	args ImportModelArgs,
) error {
	return newImportCoordinator(
		svc, deps, args.SourceMigrationUUID, args.ControllerModelInfo, export.ProjectionView{},
	).RemoveOnAbort(ctx)
}

// controllerImportOp is a single step in the v8 controller-DB import sequence.
type controllerImportOp interface {
	// Name returns the display name of this import step, used in error messages
	// and logs.
	Name() string

	// Execute performs the forward controller-DB write for this step, threading
	// any values downstream steps depend on into st.
	Execute(ctx context.Context, st *importState) error

	// RemoveOnAbort undoes the controller-DB write when the import is aborted.
	// It is idempotent and stateless: it derives what to remove from the model
	// UUID alone and is safe to call more than once.
	RemoveOnAbort(ctx context.Context) error
}

// importState is threaded forward through Execute calls to carry values that
// later steps depend on. RemoveOnAbort methods must not read it.
type importState struct {
	claimUUID     string
	inactiveUsers set.Strings
	credKey       corecredential.Key
}

// importCoordinator sequences the controller-DB import steps and exposes both
// an Import (forward) and a RemoveOnAbort (abort) driver.
type importCoordinator struct {
	ops []controllerImportOp

	// claim and modelUUID guard the sequence: see [importCoordinator.Import].
	claim     ImportClaimService
	modelUUID coremodel.UUID
}

// Import runs each op's Execute in registration order, threading importState
// forward. The first error aborts the sequence; the caller is responsible for
// calling RemoveOnAbort.
//
// Before each op that follows the claim's creation it re-asserts that the claim
// still exists and is still importing. The claim is the model's ownership
// record: if an abort takes it while an import is mid-sequence, every write
// after that point lands behind the abort's back, because the abort compensates
// the rows it knows about and anything written afterwards is left with nothing
// to remove it.
//
// This bounds the race to the single op already in flight rather than
// eliminating it. Ops whose writes must not survive an abort at all take the
// same assertion inside their own transaction, which is the only way to make
// the check and the write atomic. Model-database writes need neither, because
// abort drops the whole database and seals the namespace.
func (c *importCoordinator) Import(ctx context.Context) error {
	var st importState
	for _, op := range c.ops {
		// Only meaningful once the claim exists; the first op is what creates
		// it.
		if st.claimUUID != "" {
			if err := c.claim.AssertImporting(ctx, c.modelUUID); err != nil {
				return errors.Errorf(
					"import for model %q cannot run %q: %w", c.modelUUID, op.Name(), err)
			}
		}
		if err := op.Execute(ctx, &st); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// RemoveOnAbort runs each op's RemoveOnAbort in reverse registration order,
// collecting all errors. It is idempotent and safe to call more than once.
func (c *importCoordinator) RemoveOnAbort(ctx context.Context) error {
	var errs []error
	for i := len(c.ops) - 1; i >= 0; i-- {
		if err := c.ops[i].RemoveOnAbort(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func newImportCoordinator(
	svc ImportServices,
	deps Deps,
	sourceMigrationUUID string,
	info coremodelmigration.ControllerModelInfo,
	view export.ProjectionView,
) *importCoordinator {
	modelUUIDStr := info.ModelInfo.UUID
	modelUUID := coremodel.UUID(modelUUIDStr)

	var secretBackendName string
	if info.SecretBackend != nil {
		secretBackendName = info.SecretBackend.Name
	}
	agentStream := agentStreamFromModelConfig(view)

	ops := []controllerImportOp{
		&opBeginImport{
			claim:         svc.Claim,
			modelUUID:     modelUUID,
			modelUUIDStr:  modelUUIDStr,
			sourceMigUUID: sourceMigrationUUID,
		},
		&opImportUsers{
			access:       svc.Access,
			modelUUIDStr: modelUUIDStr,
			users:        info.Users,
		},
		&opImportCredential{
			credential:      svc.Credential,
			modelUUIDStr:    modelUUIDStr,
			modelCredential: info.ModelCredential,
		},
		&opBootstrapModel{
			deps:               deps,
			modelUUID:          modelUUID,
			modelUUIDStr:       modelUUIDStr,
			identity:           info.ModelInfo,
			secretBackendName:  secretBackendName,
			agentStream:        agentStream,
			agentTargetVersion: view.AgentTargetVersion,
		},
		&opImportExternalControllers{
			claim:        svc.Claim,
			modelUUID:    modelUUID,
			modelUUIDStr: modelUUIDStr,
			refs:         info.ExternalControllers,
		},
		&opImportPermissions{
			access:       svc.Access,
			claim:        svc.Claim,
			modelUUID:    modelUUID,
			modelUUIDStr: modelUUIDStr,
			perms:        info.Permissions,
		},
		&opImportAuthorizedKeys{
			keymanager:   svc.KeyManager,
			access:       svc.Access,
			modelUUIDStr: modelUUIDStr,
			keys:         info.AuthorizedKeys,
		},
		&opImportSecretBackendReferences{
			secretBackend: svc.SecretBackend,
			modelUUID:     modelUUID,
			modelUUIDStr:  modelUUIDStr,
			refs:          info.SecretBackendRefs,
		},
		&opImportLeadership{
			lease:        svc.Lease,
			modelUUID:    modelUUID,
			modelUUIDStr: modelUUIDStr,
			leaders:      info.Leaders,
		},
		&opImportLastLogins{
			access:       svc.Access,
			modelUUID:    modelUUID,
			modelUUIDStr: modelUUIDStr,
			users:        info.Users,
		},
		&opImportCloudImageMetadata{
			cloudImage:   svc.CloudImage,
			logger:       deps.Logger,
			modelUUIDStr: modelUUIDStr,
			metadata:     info.CloudImageMetadata,
		},
	}

	return &importCoordinator{
		ops:       ops,
		claim:     svc.Claim,
		modelUUID: modelUUID,
	}
}

// ---- per-op structs ---------------------------------------------------------

type opBeginImport struct {
	claim         ImportClaimService
	modelUUID     coremodel.UUID
	modelUUIDStr  string
	sourceMigUUID string
}

func (op *opBeginImport) Name() string { return "begin-import" }

func (op *opBeginImport) Execute(ctx context.Context, st *importState) error {
	claimUUID, err := op.claim.BeginImport(ctx, op.modelUUID, op.sourceMigUUID)
	if err != nil {
		return errors.Errorf("claiming import slot for model %q: %w", op.modelUUIDStr, err)
	}
	st.claimUUID = claimUUID
	return nil
}

// RemoveOnAbort is a no-op: the import claim is the durable anchor; Task 11
// removes it as the last step of AbortImport.
func (op *opBeginImport) RemoveOnAbort(_ context.Context) error { return nil }

// ----

type opImportUsers struct {
	access       ImportAccessService
	modelUUIDStr string
	users        []coremodelmigration.ModelUser
}

func (op *opImportUsers) Name() string { return "import-users" }

func (op *opImportUsers) Execute(ctx context.Context, st *importState) error {
	inactiveUsers, err := op.access.ImportModelUsers(ctx, op.users)
	if err != nil {
		return errors.Errorf("resolving users for model %q import: %w", op.modelUUIDStr, err)
	}
	st.inactiveUsers = inactiveUsers
	return nil
}

// RemoveOnAbort is a no-op: external users are controller-level entities shared
// across models; permissions are cleaned by opImportPermissions.RemoveOnAbort.
func (op *opImportUsers) RemoveOnAbort(_ context.Context) error { return nil }

// ----

type opImportCredential struct {
	credential      ImportCredentialService
	modelUUIDStr    string
	modelCredential *coremodelmigration.ModelCloudCredential
}

func (op *opImportCredential) Name() string { return "import-credential" }

func (op *opImportCredential) Execute(ctx context.Context, st *importState) error {
	if op.modelCredential == nil {
		return nil
	}
	credKey, err := op.credential.ImportModelCredential(ctx, *op.modelCredential)
	if err != nil {
		return errors.Errorf("resolving credential for model %q import: %w", op.modelUUIDStr, err)
	}
	st.credKey = credKey
	return nil
}

// RemoveOnAbort is a no-op: credentials are controller-level entities shared
// across models.
func (op *opImportCredential) RemoveOnAbort(_ context.Context) error { return nil }

// ----

type opBootstrapModel struct {
	deps               Deps
	modelUUID          coremodel.UUID
	modelUUIDStr       string
	identity           coremodelmigration.ModelIdentityInfo
	secretBackendName  string
	agentStream        agentbinary.AgentStream
	agentTargetVersion semversion.Number
}

func (op *opBootstrapModel) Name() string { return "bootstrap-model" }

func (op *opBootstrapModel) Execute(ctx context.Context, st *importState) error {
	if err := bootstrapImportedModel(
		ctx, op.deps, op.modelUUID, op.identity, st.credKey, op.secretBackendName,
		op.agentStream, op.agentTargetVersion,
	); err != nil {
		return errors.Errorf("bootstrapping model %q: %w", op.modelUUIDStr, err)
	}
	return nil
}

// RemoveOnAbort deletes the model row and all model-scoped controller rows.
// Idempotent: if the model was never written (Execute failed) or was already
// deleted, it returns nil.
func (op *opBootstrapModel) RemoveOnAbort(ctx context.Context) error {
	migrationSvc := modelmigrationservice.NewMigrationService(
		modelstatecontroller.NewState(op.deps.ControllerDB), op.deps.Logger,
	)
	return migrationSvc.DeleteImportedModel(ctx, op.modelUUID)
}

// ----

type opImportExternalControllers struct {
	claim        ImportClaimService
	modelUUID    coremodel.UUID
	modelUUIDStr string
	refs         []coremodelmigration.ExternalController
}

func (op *opImportExternalControllers) Name() string { return "import-external-controllers" }

func (op *opImportExternalControllers) Execute(ctx context.Context, st *importState) error {
	if err := op.claim.ImportExternalControllers(
		ctx, op.modelUUID, st.claimUUID, op.refs,
	); err != nil {
		return errors.Errorf(
			"importing external controllers for model %q import: %w", op.modelUUIDStr, err)
	}
	return nil
}

// RemoveOnAbort is a no-op: external_controller rows are shared across models
// and must not be deleted on a single-model abort.
func (op *opImportExternalControllers) RemoveOnAbort(_ context.Context) error { return nil }

// ----

type opImportPermissions struct {
	access       ImportAccessService
	claim        ImportClaimService
	modelUUID    coremodel.UUID
	modelUUIDStr string
	perms        []coremodelmigration.ModelPermission
}

func (op *opImportPermissions) Name() string { return "import-permissions" }

func (op *opImportPermissions) Execute(ctx context.Context, st *importState) error {
	offerUUIDs, err := op.access.ImportModelPermissions(ctx, op.perms, st.inactiveUsers)
	if err != nil {
		return errors.Errorf("applying permissions for model %q import: %w", op.modelUUIDStr, err)
	}
	if err := op.claim.ImportOfferPermissions(
		ctx, op.modelUUID, st.claimUUID, offerUUIDs,
	); err != nil {
		return errors.Errorf(
			"recording offer permissions for model %q import: %w", op.modelUUIDStr, err)
	}
	return nil
}

// RemoveOnAbort deletes the model-scoped and offer-scoped permission rows. Offer
// UUIDs are read back from the model_migration_import_offer companion table so
// this method is stateless.
func (op *opImportPermissions) RemoveOnAbort(ctx context.Context) error {
	offerUUIDs, err := op.claim.GetImportedOfferUUIDs(ctx, op.modelUUID)
	if err != nil {
		return errors.Errorf(
			"reading import offer UUIDs for model %q: %w", op.modelUUIDStr, err)
	}
	grantOnUUIDs := append([]string{op.modelUUIDStr}, offerUUIDs...)
	return op.access.DeletePermissionsByGrantOnUUID(ctx, grantOnUUIDs)
}

// ----

type opImportAuthorizedKeys struct {
	keymanager   ImportKeyManagerService
	access       ImportAccessService
	modelUUIDStr string
	keys         []coremodelmigration.ModelAuthorizedKey
}

func (op *opImportAuthorizedKeys) Name() string { return "import-authorized-keys" }

func (op *opImportAuthorizedKeys) Execute(ctx context.Context, st *importState) error {
	if err := op.keymanager.ImportAuthorizedKeys(
		ctx, op.keys, st.inactiveUsers, op.access.GetUserUUIDByName,
	); err != nil {
		return errors.Errorf(
			"applying authorized keys for model %q import: %w", op.modelUUIDStr, err)
	}
	return nil
}

// RemoveOnAbort deletes all authorized keys stored for the model. The keymanager
// service already treats this as idempotent.
func (op *opImportAuthorizedKeys) RemoveOnAbort(ctx context.Context) error {
	return op.keymanager.DeleteKeysForModel(ctx)
}

// ----

type opImportSecretBackendReferences struct {
	secretBackend ImportSecretBackendService
	modelUUID     coremodel.UUID
	modelUUIDStr  string
	refs          []coremodelmigration.SecretBackendReference
}

func (op *opImportSecretBackendReferences) Name() string { return "import-secret-backend-refs" }

func (op *opImportSecretBackendReferences) Execute(ctx context.Context, _ *importState) error {
	if err := op.secretBackend.ImportSecretBackendReferences(
		ctx, op.modelUUID, op.refs,
	); err != nil {
		return errors.Errorf(
			"applying secret backend references for model %q import: %w", op.modelUUIDStr, err)
	}
	return nil
}

// RemoveOnAbort is a no-op: secret_backend_reference rows are covered by the
// model.Delete cascade in opBootstrapModel.RemoveOnAbort.
func (op *opImportSecretBackendReferences) RemoveOnAbort(_ context.Context) error { return nil }

// ----

type opImportLeadership struct {
	lease        ImportLeaseService
	modelUUID    coremodel.UUID
	modelUUIDStr string
	leaders      []coremodelmigration.ApplicationLeadership
}

func (op *opImportLeadership) Name() string { return "import-leadership" }

func (op *opImportLeadership) Execute(ctx context.Context, _ *importState) error {
	if err := op.lease.ImportApplicationLeadership(
		ctx, op.modelUUID, op.leaders,
	); err != nil {
		return errors.Errorf(
			"claiming leadership leases for model %q import: %w", op.modelUUIDStr, err)
	}
	return nil
}

// RemoveOnAbort deletes all application-leadership leases for the model.
func (op *opImportLeadership) RemoveOnAbort(ctx context.Context) error {
	return op.lease.DeleteLeadershipForModel(ctx, op.modelUUID)
}

// ----

type opImportLastLogins struct {
	access       ImportAccessService
	modelUUID    coremodel.UUID
	modelUUIDStr string
	users        []coremodelmigration.ModelUser
}

func (op *opImportLastLogins) Name() string { return "import-last-logins" }

func (op *opImportLastLogins) Execute(ctx context.Context, st *importState) error {
	if err := op.access.ImportLastModelLogins(
		ctx, op.modelUUID, op.users, st.inactiveUsers,
	); err != nil {
		return errors.Errorf(
			"applying last logins for model %q import: %w", op.modelUUIDStr, err)
	}
	return nil
}

// RemoveOnAbort is a no-op: model_last_login rows are covered by the
// model.Delete cascade in opBootstrapModel.RemoveOnAbort.
func (op *opImportLastLogins) RemoveOnAbort(_ context.Context) error { return nil }

// ----

type opImportCloudImageMetadata struct {
	cloudImage   ImportCloudImageMetadataService
	logger       logger.Logger
	modelUUIDStr string
	metadata     []coremodelmigration.CloudImageMetadata
}

func (op *opImportCloudImageMetadata) Name() string { return "import-cloud-image-metadata" }

func (op *opImportCloudImageMetadata) Execute(ctx context.Context, _ *importState) error {
	imageConflicts, err := op.cloudImage.ImportCloudImageMetadata(ctx, op.metadata)
	if err != nil {
		return errors.Errorf(
			"applying cloud image metadata for model %q import: %w", op.modelUUIDStr, err)
	}
	for _, conflict := range imageConflicts {
		// Non-fatal: the target's custom image metadata is kept (target-wins);
		// the operator can reconcile via the normal custom-metadata path.
		op.logger.Warningf(ctx,
			"model %q import: keeping target custom cloud image metadata for %s/%s/%s/%s image %q, skipping source image %q",
			op.modelUUIDStr, conflict.Stream, conflict.Region, conflict.Version,
			conflict.Arch, conflict.ExistingImageID, conflict.IncomingImageID)
	}
	return nil
}

// RemoveOnAbort is a no-op: cloud image metadata is controller-scoped and
// shared; target-wins semantics mean no rollback is needed.
func (op *opImportCloudImageMetadata) RemoveOnAbort(_ context.Context) error { return nil }

// ---- services bundle --------------------------------------------------------

// importServices bundles the controller-scoped domain services the v8 import
// driver orchestrates. They are constructed once at the start of
// ImportControllerModelInfo and shared across the import steps.
// ImportServices are the controller-scoped domain services the v8 import driver
// calls. They are constructed above the coordinator rather than inside it, so
// that a caller - production or test - decides what the import writes through.
type ImportServices struct {
	Claim         ImportClaimService
	Access        ImportAccessService
	Credential    ImportCredentialService
	KeyManager    ImportKeyManagerService
	SecretBackend ImportSecretBackendService
	Lease         ImportLeaseService
	CloudImage    ImportCloudImageMetadataService
}

// newImportServices constructs the controller-scoped domain services the v8
// import driver needs. Each service owns its state and is independent of the
// others; the import driver is responsible for calling them in FK-safe order.
// NewImportServices constructs the real controller-scoped services from the
// database handles in deps. Each service owns its own state and is independent
// of the others; the import driver is responsible for calling them in FK-safe
// order.
func NewImportServices(deps Deps, modelUUID coremodel.UUID) ImportServices {
	return ImportServices{
		Claim: migrationclaimservice.NewImportService(
			migrationclaimstate.New(deps.ControllerDB, deps.Clock), deps.Logger,
		),
		Access: accessservice.NewService(
			accessstate.NewState(deps.ControllerDB, deps.Clock, deps.Logger), deps.Clock,
		),
		Credential: credentialservice.NewService(
			credentialstate.NewState(deps.ControllerDB), deps.Logger,
		),
		KeyManager: keymanagerservice.NewService(
			modelUUID, keymanagerstate.NewState(deps.ControllerDB),
		),
		SecretBackend: secretbackendservice.NewService(
			secretbackendstate.NewState(deps.ControllerDB, deps.Logger), deps.Logger,
		),
		Lease: leaseservice.NewService(
			leasestate.NewState(deps.ControllerDB, deps.Logger),
		),
		CloudImage: cloudimagemetadataservice.NewService(
			cloudimagemetadatastate.NewState(deps.ControllerDB, deps.Clock, deps.Logger),
		),
	}
}

// ---- helpers ----------------------------------------------------------------

// bootstrapImportedModel creates the controller-database model row (claim-free:
// the v8 import claim is owned by the modelmigration domain, not this call) and
// then establishes the model database's read-only model info, marking it as
// importing so charm uploads during the migration are handled correctly. It is
// pure orchestration of two existing model-domain service methods.
func bootstrapImportedModel(
	ctx context.Context,
	deps Deps,
	modelUUID coremodel.UUID,
	identity coremodelmigration.ModelIdentityInfo,
	credKey corecredential.Key,
	secretBackendName string,
	agentStream agentbinary.AgentStream,
	agentTargetVersion semversion.Number,
) error {
	migrationSvc := modelmigrationservice.NewMigrationService(
		modelstatecontroller.NewState(deps.ControllerDB), deps.Logger,
	)
	modelSvc := modelservice.NewModelService(
		modelUUID,
		modelstatecontroller.NewState(deps.ControllerDB),
		modelstatemodel.NewState(deps.ModelDB, deps.Logger),
		modelservice.EnvironVersionProviderGetter(),
		modelservice.DefaultAgentBinaryFinder(),
	)

	args := domainmodel.ModelImportArgs{
		UUID: modelUUID,
		GlobalModelCreationArgs: domainmodel.GlobalModelCreationArgs{
			Cloud:         identity.Cloud,
			CloudRegion:   identity.CloudRegion,
			Credential:    credKey,
			Name:          identity.Name,
			Qualifier:     coremodel.Qualifier(identity.Qualifier),
			SecretBackend: secretBackendName,
		},
	}

	if err := migrationSvc.ImportModel(ctx, args); err != nil {
		return errors.Errorf("creating model %q: %w", identity.Name, err)
	}
	if err := modelSvc.CreateImportingModelWithAgentVersionStream(ctx, agentTargetVersion, agentStream); err != nil {
		return errors.Errorf("creating model %q database: %w", identity.Name, err)
	}
	return nil
}

// agentStreamFromModelConfig reads the model's configured agent stream out of
// the projection view, defaulting to the released stream when unset.
func agentStreamFromModelConfig(view export.ProjectionView) agentbinary.AgentStream {
	if view.AgentStream != "" {
		return agentbinary.AgentStream(view.AgentStream)
	}
	return agentbinary.AgentStreamReleased
}
