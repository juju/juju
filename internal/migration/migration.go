// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/juju/clock"
	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/naturalsort"

	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/modeldefaults"
	migrations "github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/tools"
)

// OperationExporter describes the interface for running the ExportOpertions
// method.
type OperationExporter interface {
	// ExportOperations registers the export operations with the given coordinator.
	ExportOperations(registry corestorage.ModelStorageRegistryGetter)
}

// Coordinator describes the interface required for coordinating model
// migration operations.
type Coordinator interface {
	// Add a new operation to the migration. It will be appended at the end of the
	// list of operations.
	Add(operations modelmigration.Operation)
	// Perform executes the migration.
	// We log in addition to returning errors because the error is ultimately
	// returned to the caller on the source, and we want them to be reflected
	// in *this* controller's logs.
	Perform(ctx context.Context, scope modelmigration.Scope, model description.Model) (err error)
}

// ModelExporter facilitates partial and full export of a model.
type ModelExporter struct {
	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	operationExporter     OperationExporter

	scope       modelmigration.Scope
	coordinator Coordinator
	logger      corelogger.Logger

	clock clock.Clock
}

// NewModelExporter returns a new ModelExporter that encapsulates the
// legacyStateExporter. The legacyStateExporter is being deprecated, only
// needed until the migration to dqlite is complete.
func NewModelExporter(
	operationExporter OperationExporter,
	scope modelmigration.Scope,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	coordinator Coordinator,
	logger corelogger.Logger,
	clock clock.Clock,
) *ModelExporter {
	me := &ModelExporter{
		operationExporter:     operationExporter,
		scope:                 scope,
		storageRegistryGetter: storageRegistryGetter,
		coordinator:           coordinator,
		logger:                logger,
		clock:                 clock,
	}
	me.operationExporter.ExportOperations(me.storageRegistryGetter)
	return me
}

// ExportModel serializes a model description from the database (legacy mongodb
// plus dqlite) contents.
func (e *ModelExporter) ExportModel(ctx context.Context, store objectstore.ObjectStore) (description.Model, error) {
	var model description.Model
	if model == nil {
		return nil, errors.ConstError("model export not implemented")
	}
	return e.Export(ctx, model)
}

// Export serializes a model description from the database contents.
func (e *ModelExporter) Export(ctx context.Context, model description.Model) (description.Model, error) {
	if err := e.coordinator.Perform(ctx, e.scope, model); err != nil {
		return nil, errors.Trace(err)
	}
	// The model now contains all the exported data from the legacy state along
	// with the new domains' one. Time to validate.
	if err := model.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return model, nil
}

// ConfigSchemaSourceProvider returns a config.ConfigSchemaSourceGetter based
// on the given cloud service.
type ConfigSchemaSourceProvider = func(environs.CloudService) config.ConfigSchemaSourceGetter

// ModelImporter represents a model migration that implements Import.
type ModelImporter struct {
	controllerConfigService ControllerConfigService
	domainServices          services.DomainServicesGetter
	storageRegistryGetter   corestorage.ModelStorageRegistryGetter
	objectStoreGetter       objectstore.ModelObjectStoreGetter

	scope  modelmigration.ScopeForModel
	logger corelogger.Logger
	clock  clock.Clock
}

// NewModelImporter returns a new ModelImporter that encapsulates the
// legacyStateImporter. The legacyStateImporter is being deprecated, only
// needed until the migration to dqlite is complete.
func NewModelImporter(
	scope modelmigration.ScopeForModel,
	controllerConfigService ControllerConfigService,
	domainServices services.DomainServicesGetter,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
	logger corelogger.Logger,
	clock clock.Clock,
) *ModelImporter {
	return &ModelImporter{
		scope:                   scope,
		controllerConfigService: controllerConfigService,
		domainServices:          domainServices,
		storageRegistryGetter:   storageRegistryGetter,
		objectStoreGetter:       objectStoreGetter,
		logger:                  logger,
		clock:                   clock,
	}
}

// ImportModel deserializes a model description from the bytes, transforms
// the model config based on information from the controller model, and then
// imports that as a new database model.
func (i *ModelImporter) ImportModel(ctx context.Context, bytes []byte) error {
	model, err := description.Deserialize(bytes)
	if err != nil {
		return errors.Trace(err)
	}

	modelUUID := coremodel.UUID(model.UUID())

	// The domain services are not available during the import, until the
	// model is created and activated. The model defaults provider is used
	// to provide the model defaults during the migration, so we allow access
	// but in a lazy way.

	modelDefaultsProvider := modelDefaultsProvider{
		modelUUID:      modelUUID,
		servicesGetter: i.domainServices,
	}

	coordinator := modelmigration.NewCoordinator(i.logger)
	migrations.ImportOperations(coordinator, modelDefaultsProvider, i.storageRegistryGetter, i.objectStoreGetter, i.clock, i.logger)
	if err := coordinator.Perform(ctx, i.scope(modelUUID), model); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type modelDefaultsProvider struct {
	modelUUID      coremodel.UUID
	servicesGetter services.DomainServicesGetter
}

func (p modelDefaultsProvider) ModelDefaults(ctx context.Context) (modeldefaults.Defaults, error) {
	domainServices, err := p.servicesGetter.ServicesForModel(ctx, p.modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelDefaults := domainServices.ModelDefaults()
	fn := modelDefaults.ModelDefaultsProvider(p.modelUUID)
	return fn(ctx)
}

type CharmService interface {
	// GetCharmArchive returns a ReadCloser stream for the charm archive for a given
	// charm id, along with the hash of the charm archive. Clients can use the hash
	// to verify the integrity of the charm archive.
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
	GetAgentBinaryForSHA256(context.Context, string) (io.ReadCloser, int64, error)
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
	}
	return nil
}

func uploadTools(
	ctx context.Context,
	config UploadBinariesConfig,
	logger corelogger.Logger,
) error {
	for sha256Sum, version := range config.Tools {
		logger.Debugf(
			ctx,
			"sending agent binaries for sha256 %q and version %q to target controller",
			sha256Sum, version,
		)

		reader, _, err := config.AgentBinaryStore.GetAgentBinaryForSHA256(ctx, sha256Sum)
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
				"upladoing agent binaries for sha256 %q and version %q: %w",
				sha256Sum, version, err,
			)
		}
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
