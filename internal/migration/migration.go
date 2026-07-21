// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/juju/clock"
	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/export"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelimport"
	migrationclaimservice "github.com/juju/juju/domain/modelmigration/service"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/migration/legacy"
	"github.com/juju/juju/internal/naturalsort"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/tools"
)

// OperationExporter describes the interface for running the ExportOpertions
// method.
type OperationExporter interface {
	// ExportOperations registers the export operations with the given coordinator.
	ExportOperations(registry corestorage.ModelStorageRegistryGetter)
}

// ConfigSchemaSourceProvider returns a config.ConfigSchemaSourceGetter based
// on the given cloud service.
type ConfigSchemaSourceProvider = func(environs.CloudService) config.ConfigSchemaSourceGetter

// ModelImporter represents a model migration that implements Import.
type ModelImporter struct {
	domainServices services.DomainServicesGetter

	controllerUUID string
	scope          modelmigration.ScopeForModel
	logger         corelogger.Logger
	clock          clock.Clock
}

// NewModelImporter returns a new ModelImporter that encapsulates the
// legacyStateImporter. The legacyStateImporter is being deprecated, only
// needed until the migration to dqlite is complete.
func NewModelImporter(
	scope modelmigration.ScopeForModel,
	domainServices services.DomainServicesGetter,
	controllerUUID string,
	logger corelogger.Logger,
	clock clock.Clock,
) *ModelImporter {
	return &ModelImporter{
		scope:          scope,
		controllerUUID: controllerUUID,
		domainServices: domainServices,
		logger:         logger,
		clock:          clock,
	}
}

// ImportModelLegacy deserializes a legacy description model from the bytes,
// transforms the model config based on information from the controller model,
// and imports that as a new database model.
func (i *ModelImporter) ImportModelLegacy(ctx context.Context, bytes []byte) error {
	return legacy.ImportModel(
		ctx, bytes, i.scope, i.domainServices, i.controllerUUID, i.logger, i.clock,
	)
}

// ActivateModel finalises the activation of a model imported via the v8 path.
// It resolves the domain services for args.ModelUUID and delegates to the
// activation driver.
func (i *ModelImporter) ActivateModel(ctx context.Context, args ActivateModelArgs) error {
	domainServices, err := i.domainServices.ServicesForModel(ctx, args.ModelUUID)
	if err != nil {
		return internalerrors.Errorf(
			"retrieving domain services for model %q: %w", args.ModelUUID, err,
		)
	}
	return activateModel(ctx, domainServices, args)
}

// AbortModel drives target-side cleanup of a partially imported v8 model. It
// resolves the controller-database scope for the model UUID and delegates to
// [AbortModelImport], then blocks (via [WaitAbortFinalized]) until the model
// database has been dropped and the import claim released, so the model UUID is
// free when this returns and an immediate re-migration succeeds. The model
// database is never opened during abort, so no model-DB scope is needed.
//
// The controller-scoped modelmigration import service is constructed directly
// from the controller DB rather than via ServicesForModel: the abort path only
// uses controller-DB state (import claims, namespace registrations, staged
// deletions), and a re-invocation after a prior partial abort may have already
// removed the model namespace, which would make ServicesForModel fail.
//
// If the claim cannot be finalized within the wait budget the abort is still
// accepted: the claim stays in the aborting phase and the abort reconciler
// completes it later.
//
// It returns an error wrapping
// [github.com/juju/juju/domain/modelmigration/errors.ErrAbortActivating] when
// activation has already crossed the point of no return.
func (i *ModelImporter) AbortModel(ctx context.Context, modelUUID coremodel.UUID) error {
	scope := i.scope(modelUUID)
	deps := Deps{
		ControllerDB: scope.ControllerDB(),
		Clock:        i.clock,
		Logger:       i.logger,
	}
	claim := migrationclaimservice.NewImportService(
		migrationclaimstate.New(deps.ControllerDB, deps.Clock), deps.Logger,
	)
	if err := AbortModelImport(ctx, deps, claim, modelUUID); err != nil {
		return err
	}
	return WaitAbortFinalized(ctx, deps, claim, modelUUID, DefaultAbortFinalizeWait)
}

// ImportModel applies a v8 import's controller-scoped semantic data to the
// target controller. See [ImportControllerModelInfo] for the orchestration;
// this method only resolves the migration scope for the model UUID and
// delegates.
//
// If a claim already exists for args.ControllerModelInfo.ModelInfo.UUID, the
// returned error wraps [coreerrors.AlreadyExists] (phase-specific wording is
// supplied by the modelmigration domain).
func (i *ModelImporter) ImportModel(
	ctx context.Context, args ImportModelArgs, view export.ProjectionView,
) error {
	modelUUID := coremodel.UUID(args.ControllerModelInfo.ModelInfo.UUID)
	scope := i.scope(modelUUID)
	deps := Deps{
		ControllerDB: scope.ControllerDB(),
		ModelDB:      scope.ModelDB(),
		Clock:        i.clock,
		Logger:       i.logger,
	}

	// Apply the controller-scoped data (claim, bootstrap, users, credential,
	// permissions, secret backend references, ...). Writes only; no return
	// value beyond the error.
	if err := ImportControllerModelInfo(
		ctx, deps, args.SourceMigrationUUID, args.ControllerModelInfo, view,
	); err != nil {
		return internalerrors.Capture(err)
	}

	if args.ModelDBPayload != nil {
		// Now that the controller data is applied, rewrite the model-DB payload's
		// live secret value ref backend UUIDs from the source controller's to the
		// target's (matched by name) before the insert. An unmapped live revision
		// is a hard error; no model-DB rows are written.
		if err := reconcileSecretBackendUUIDs(ctx, deps, args.ControllerModelInfo, args.ModelDBPayload); err != nil {
			return internalerrors.Errorf(
				"rewriting secret backend UUIDs for model %q: %w", modelUUID, err)
		}

		// Model-DB import: insert the transformed, target-version payload into the
		// model DB. The importer is constructed per import because it binds to the
		// model DB resolved from the scope for this model UUID (the ModelImporter
		// itself is not bound to a single model).
		if err := modelimport.NewImporter(scope.ModelDB()).Import(ctx, args.ModelDBPayload); err != nil {
			return internalerrors.Errorf("model-DB import for model %q: %w", modelUUID, err)
		}
	}

	// Activate the imported model so it is visible in v_model and connectable by
	// the migrating model's agents during the source VALIDATION phase. The model
	// stays gated by the model_migrating flag and the importing import claim until
	// the target Activate call clears them; activation here only flips
	// model.activated, mirroring the legacy importModelActivatorOperation. This is
	// idempotent: a retried import (or the later Activate) tolerates
	// AlreadyActivated.
	modelDomainServices, err := i.domainServices.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return internalerrors.Errorf("retrieving domain services for model %q: %w", modelUUID, err)
	}
	if err := modelDomainServices.Model().ActivateModel(ctx, modelUUID); err != nil &&
		!internalerrors.Is(err, modelerrors.AlreadyActivated) {
		return internalerrors.Errorf("activating imported model %q: %w", modelUUID, err)
	}
	return nil
}

type CharmService interface {
	// GetCharmArchive returns a ReadCloser stream for the charm archive for a
	// given charm id, along with the hash of the charm archive. Clients can use
	// the hash to verify the integrity of the charm archive.
	GetCharmArchive(context.Context, domaincharm.CharmLocator) (io.ReadCloser, string, error)
}

// CharmUploader defines a single method that is used to upload a
// charm to the target controller in a migration.
type CharmUploader interface {
	UploadCharm(ctx context.Context, charmURL string, charmRef string, content io.Reader) (string, error)
}

// AgentBinaryStore provides an interface for interacting with the stored agent
// binaries within a controller and model.
type AgentBinaryStore interface {
	// GetAgentBinaryForSHA256 returns the agent binary associated with the
	// given SHA256 sum. The following errors can be expected:
	// - [github.com/juju/juju/domain/agentbinary/errors.NotFound] when no agent
	// binaries exist for the provided sha.
	GetAgentBinaryUsingSHA256(context.Context, string) (io.ReadCloser, int64, error)
}

// ToolsUploader defines a single method that is used to upload tools
// to the target controller in a migration.
type ToolsUploader interface {
	UploadTools(context.Context, io.Reader, semversion.Binary) (tools.List, error)
}

// ResourceDownloader defines the interface for downloading resources
// from the source controller during a migration.
type ResourceDownloader interface {
	OpenResource(context.Context, string, string) (io.ReadCloser, error)
}

// ResourceUploader defines the interface for uploading resources into
// the target controller during a migration.
type ResourceUploader interface {
	UploadResource(context.Context, resource.Resource, io.Reader) error
}

// UploadBinariesConfig provides all the configuration that the
// UploadBinaries function needs to operate. To construct the config
// with the default helper functions, use `NewUploadBinariesConfig`.
type UploadBinariesConfig struct {
	Charms        []string
	CharmService  CharmService
	CharmUploader CharmUploader

	// Tools is a collection of agent binaries to be uploaded keyed on the
	// sha256 sum and referenced to a version.
	Tools            map[string]semversion.Binary
	AgentBinaryStore AgentBinaryStore
	ToolsUploader    ToolsUploader

	Resources          []resource.Resource
	ResourceDownloader ResourceDownloader
	ResourceUploader   ResourceUploader
}

// Validate makes sure that all the config values are non-nil.
func (c *UploadBinariesConfig) Validate() error {
	if c.CharmService == nil {
		return errors.NotValidf("missing CharmService")
	}
	if c.CharmUploader == nil {
		return errors.NotValidf("missing CharmUploader")
	}
	if c.AgentBinaryStore == nil {
		return errors.NotValidf("missing AgentBinaryStore")
	}
	if c.ToolsUploader == nil {
		return errors.NotValidf("missing ToolsUploader")
	}
	if c.ResourceDownloader == nil {
		return errors.NotValidf("missing ResourceDownloader")
	}
	if c.ResourceUploader == nil {
		return errors.NotValidf("missing ResourceUploader")
	}
	return nil
}

// UploadBinaries will send binaries stored in the source blobstore to
// the target controller.
func UploadBinaries(ctx context.Context, config UploadBinariesConfig, logger corelogger.Logger) error {
	if err := config.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := uploadCharms(ctx, config, logger); err != nil {
		return errors.Annotatef(err, "cannot upload charms")
	}
	if err := uploadTools(ctx, config, logger); err != nil {
		return errors.Annotatef(err, "cannot upload agent binaries")
	}
	if err := uploadResources(ctx, config, logger); err != nil {
		return errors.Annotatef(err, "cannot upload resources")
	}
	return nil
}

func streamThroughTempFile(r io.Reader) (_ io.ReadSeeker, cleanup func(), err error) {
	tempFile, err := os.CreateTemp("", "juju-migrate-binary")
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempFile.Name())
		}
	}()
	_, err = io.Copy(tempFile, r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	_, err = tempFile.Seek(0, 0)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "potentially corrupt binary")
	}
	rmTempFile := func() {
		filename := tempFile.Name()
		_ = tempFile.Close()
		_ = os.Remove(filename)
	}

	return tempFile, rmTempFile, nil
}

func uploadCharms(ctx context.Context, config UploadBinariesConfig, logger corelogger.Logger) error {
	// It is critical that charms are uploaded in ascending charm URL
	// order so that charm revisions end up the same in the target as
	// they were in the source.
	naturalsort.Sort(config.Charms)

	for _, charmURL := range config.Charms {
		if err := uploadCharm(ctx, config, logger, charmURL); err != nil {
			return err
		}
	}
	return nil
}

func uploadCharm(
	ctx context.Context,
	config UploadBinariesConfig,
	logger corelogger.Logger,
	charmURL string,
) error {
	logger.Debugf(ctx, "sending charm %s to target", charmURL)
	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return errors.Annotate(err, "bad charm URL")
	}
	charmSource, err := domaincharm.ParseCharmSchema(charm.Schema(curl.Schema))
	if err != nil {
		return errors.Annotate(err, "bad charm URL schema")
	}
	reader, hash, err := config.CharmService.GetCharmArchive(ctx, domaincharm.CharmLocator{
		Name:     curl.Name,
		Revision: curl.Revision,
		Source:   charmSource,
	})
	if err != nil {
		return errors.Annotate(err, "cannot open charm")
	}
	defer func() { _ = reader.Close() }()

	charmRef := fmt.Sprintf("%s-%s", curl.Name, hash[0:8])
	if usedCurl, err := config.CharmUploader.UploadCharm(ctx, charmURL, charmRef, reader); err != nil {
		return errors.Annotate(err, "cannot upload charm")
	} else if usedCurl != charmURL {
		// The target controller shouldn't assign a different charm URL.
		return errors.Errorf("charm %s unexpectedly assigned %s", charmURL, usedCurl)
	}
	return nil
}

func uploadTools(
	ctx context.Context,
	config UploadBinariesConfig,
	logger corelogger.Logger,
) error {
	for sha256Sum, version := range config.Tools {
		if err := uploadTool(ctx, config, logger, sha256Sum, version); err != nil {
			return err
		}
	}
	return nil
}

func uploadTool(
	ctx context.Context,
	config UploadBinariesConfig,
	logger corelogger.Logger,
	sha256Sum string,
	version semversion.Binary,
) error {
	logger.Debugf(
		ctx,
		"sending agent binaries for sha256 %q and version %q to target controller",
		sha256Sum, version,
	)

	reader, _, err := config.AgentBinaryStore.GetAgentBinaryUsingSHA256(ctx, sha256Sum)
	if err != nil {
		return internalerrors.Errorf(
			"getting agent binaries for sha %q to upload in migration: %w",
			sha256Sum, err,
		)
	}
	defer func() { _ = reader.Close() }()

	_, err = config.ToolsUploader.UploadTools(ctx, reader, version)
	if err != nil {
		return internalerrors.Errorf(
			"uploading agent binaries for sha256 %q and version %q: %w",
			sha256Sum, version, err,
		)
	}
	return nil
}

func uploadResources(ctx context.Context, config UploadBinariesConfig, logger corelogger.Logger) error {
	for _, res := range config.Resources {
		if !res.Fingerprint.IsZero() {
			// If the fingerprint is zero there is no blob to upload for this
			// resource, skip it.
			err := uploadAppResource(ctx, config, res, logger)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func uploadAppResource(ctx context.Context, config UploadBinariesConfig, rev resource.Resource, logger corelogger.Logger) error {
	logger.Debugf(ctx, "opening application resource for %s: %s", rev.ApplicationName, rev.Name)
	reader, err := config.ResourceDownloader.OpenResource(ctx, rev.ApplicationName, rev.Name)
	if err != nil {
		return errors.Annotate(err, "cannot open resource")
	}
	defer func() { _ = reader.Close() }()

	// TODO(menn0) - validate that the downloaded revision matches
	// the expected metadata. Check revision and fingerprint.

	content, cleanup, err := streamThroughTempFile(reader)
	if err != nil {
		return errors.Trace(err)
	}
	defer cleanup()

	if err := config.ResourceUploader.UploadResource(ctx, rev, content); err != nil {
		return errors.Annotate(err, "cannot upload resource")
	}
	return nil
}
