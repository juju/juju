// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/juju/charm/v13"
	"github.com/juju/description/v5"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/naturalsort"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resources"
	migrations "github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLoggerWithTags("juju.migration", corelogger.MIGRATION)

// LegacyStateExporter describes interface on state required to export a
// model.
// Deprecated: This is being replaced with the ModelExporter.
type LegacyStateExporter interface {
	// Export generates an abstract representation of a model.
	Export(leaders map[string]string, store objectstore.ObjectStore) (description.Model, error)
	// ExportPartial produces a partial export based based on the input
	// config.
	ExportPartial(cfg state.ExportConfig, store objectstore.ObjectStore) (description.Model, error)
}

// ModelExporter facilitates partial and full export of a model.
type ModelExporter struct {
	// TODO(nvinuesa): This is being deprecated, only needed until the
	// migration to dqlite is complete.
	legacyStateExporter LegacyStateExporter

	scope modelmigration.Scope
}

// NewModelExporter returns a new ModelExporter that encapsulates the
// legacyStateExporter. The legacyStateExporter is being deprecated, only
// needed until the migration to dqlite is complete.
func NewModelExporter(legacyStateExporter LegacyStateExporter, scope modelmigration.Scope) *ModelExporter {
	return &ModelExporter{legacyStateExporter: legacyStateExporter, scope: scope}
}

// ExportModelPartial partially serializes a model description from the
// database (legacy mongodb plus dqlite) contents, optionally skipping aspects
// as defined by the ExportConfig.
func (e *ModelExporter) ExportModelPartial(ctx context.Context, cfg state.ExportConfig, store objectstore.ObjectStore) (description.Model, error) {
	model, err := e.legacyStateExporter.ExportPartial(cfg, store)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return e.Export(ctx, model)
}

// ExportModel serializes a model description from the database (legacy mongodb
// plus dqlite) contents.
func (e *ModelExporter) ExportModel(ctx context.Context, leaders map[string]string, store objectstore.ObjectStore) (description.Model, error) {
	model, err := e.legacyStateExporter.Export(leaders, store)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return e.Export(ctx, model)
}

// Export serializes a model description from the database contents.
func (e *ModelExporter) Export(ctx context.Context, model description.Model) (description.Model, error) {
	coordinator := modelmigration.NewCoordinator()
	migrations.ExportOperations(coordinator)
	if err := coordinator.Perform(ctx, e.scope, model); err != nil {
		return nil, errors.Trace(err)
	}

	return model, nil
}

// Note: This is being deprecated.
// legacyStateImporter describes the method needed to import a model
// into the database.
type legacyStateImporter interface {
	Import(description.Model, controller.Config) (*state.Model, *state.State, error)
}

// ModelImporter represents a model migration that implements Import.
type ModelImporter struct {
	// TODO(nvinuesa): This is being deprecated, only needed until the
	// migration to dqlite is complete.
	legacyStateImporter     legacyStateImporter
	controllerConfigService ControllerConfigService
	serviceFactoryGetter    servicefactory.ServiceFactoryGetter

	scope modelmigration.ScopeForModel
}

// NewModelImporter returns a new ModelImporter that encapsulates the
// legacyStateImporter. The legacyStateImporter is being deprecated, only
// needed until the migration to dqlite is complete.
func NewModelImporter(
	stateImporter legacyStateImporter,
	scope modelmigration.ScopeForModel,
	controllerConfigService ControllerConfigService,
	serviceFactoryGetter servicefactory.ServiceFactoryGetter,
) *ModelImporter {
	return &ModelImporter{
		legacyStateImporter:     stateImporter,
		scope:                   scope,
		controllerConfigService: controllerConfigService,
		serviceFactoryGetter:    serviceFactoryGetter,
	}
}

// ImportModel deserializes a model description from the bytes, transforms
// the model config based on information from the controller model, and then
// imports that as a new database model.
func (i *ModelImporter) ImportModel(ctx context.Context, bytes []byte) (*state.Model, *state.State, error) {
	model, err := description.Deserialize(bytes)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	ctrlConfig, err := i.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "unable to get controller config")
	}

	dbModel, dbState, err := i.legacyStateImporter.Import(model, ctrlConfig)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	coordinator := modelmigration.NewCoordinator()
	migrations.ImportOperations(coordinator, logger)
	if err := coordinator.Perform(ctx, i.scope(model.Tag().Id()), model); err != nil {
		return nil, nil, errors.Trace(err)
	}

	return dbModel, dbState, nil
}

// CharmDownloader defines a single method that is used to download a
// charm from the source controller in a migration.
type CharmDownloader interface {
	OpenCharm(context.Context, string) (io.ReadCloser, error)
}

// CharmUploader defines a single method that is used to upload a
// charm to the target controller in a migration.
type CharmUploader interface {
	UploadCharm(ctx context.Context, charmURL string, charmRef string, content io.ReadSeeker) (string, error)
}

// ToolsDownloader defines a single method that is used to download
// tools from the source controller in a migration.
type ToolsDownloader interface {
	OpenURI(context.Context, string, url.Values) (io.ReadCloser, error)
}

// ToolsUploader defines a single method that is used to upload tools
// to the target controller in a migration.
type ToolsUploader interface {
	UploadTools(context.Context, io.ReadSeeker, version.Binary) (tools.List, error)
}

// ResourceDownloader defines the interface for downloading resources
// from the source controller during a migration.
type ResourceDownloader interface {
	OpenResource(context.Context, string, string) (io.ReadCloser, error)
}

// ResourceUploader defines the interface for uploading resources into
// the target controller during a migration.
type ResourceUploader interface {
	UploadResource(context.Context, resources.Resource, io.ReadSeeker) error
	SetPlaceholderResource(context.Context, resources.Resource) error
	SetUnitResource(context.Context, string, resources.Resource) error
}

// UploadBinariesConfig provides all the configuration that the
// UploadBinaries function needs to operate. To construct the config
// with the default helper functions, use `NewUploadBinariesConfig`.
type UploadBinariesConfig struct {
	Charms          []string
	CharmDownloader CharmDownloader
	CharmUploader   CharmUploader

	Tools           map[version.Binary]string
	ToolsDownloader ToolsDownloader
	ToolsUploader   ToolsUploader

	Resources          []migration.SerializedModelResource
	ResourceDownloader ResourceDownloader
	ResourceUploader   ResourceUploader
}

// Validate makes sure that all the config values are non-nil.
func (c *UploadBinariesConfig) Validate() error {
	if c.CharmDownloader == nil {
		return errors.NotValidf("missing CharmDownloader")
	}
	if c.CharmUploader == nil {
		return errors.NotValidf("missing CharmUploader")
	}
	if c.ToolsDownloader == nil {
		return errors.NotValidf("missing ToolsDownloader")
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
func UploadBinaries(ctx context.Context, config UploadBinariesConfig) error {
	if err := config.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := uploadCharms(ctx, config); err != nil {
		return errors.Annotatef(err, "cannot upload charms")
	}
	if err := uploadTools(ctx, config); err != nil {
		return errors.Annotatef(err, "cannot upload agent binaries")
	}
	if err := uploadResources(ctx, config); err != nil {
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

func hashArchive(archive io.ReadSeeker) (string, error) {
	hash := sha256.New()
	_, err := io.Copy(hash, archive)
	if err != nil {
		return "", errors.Trace(err)
	}
	_, err = archive.Seek(0, os.SEEK_SET)
	if err != nil {
		return "", errors.Trace(err)
	}
	return hex.EncodeToString(hash.Sum(nil))[0:7], nil
}

func uploadCharms(ctx context.Context, config UploadBinariesConfig) error {
	// It is critical that charms are uploaded in ascending charm URL
	// order so that charm revisions end up the same in the target as
	// they were in the source.
	naturalsort.Sort(config.Charms)

	for _, charmURL := range config.Charms {
		logger.Debugf("sending charm %s to target", charmURL)
		reader, err := config.CharmDownloader.OpenCharm(ctx, charmURL)
		if err != nil {
			return errors.Annotate(err, "cannot open charm")
		}
		defer func() { _ = reader.Close() }()

		content, cleanup, err := streamThroughTempFile(reader)
		if err != nil {
			return errors.Trace(err)
		}
		defer cleanup()

		curl, err := charm.ParseURL(charmURL)
		if err != nil {
			return errors.Annotate(err, "bad charm URL")
		}
		hash, err := hashArchive(content)
		if err != nil {
			return errors.Trace(err)
		}
		charmRef := fmt.Sprintf("%s-%s", curl.Name, hash)

		if usedCurl, err := config.CharmUploader.UploadCharm(ctx, charmURL, charmRef, content); err != nil {
			return errors.Annotate(err, "cannot upload charm")
		} else if usedCurl != charmURL {
			// The target controller shouldn't assign a different charm URL.
			return errors.Errorf("charm %s unexpectedly assigned %s", charmURL, usedCurl)
		}
	}
	return nil
}

func uploadTools(ctx context.Context, config UploadBinariesConfig) error {
	for v, uri := range config.Tools {
		logger.Debugf("sending agent binaries to target: %s", v)

		reader, err := config.ToolsDownloader.OpenURI(ctx, uri, nil)
		if err != nil {
			return errors.Annotate(err, "cannot open charm")
		}
		defer func() { _ = reader.Close() }()

		content, cleanup, err := streamThroughTempFile(reader)
		if err != nil {
			return errors.Trace(err)
		}
		defer cleanup()

		if _, err := config.ToolsUploader.UploadTools(context.TODO(), content, v); err != nil {
			return errors.Annotate(err, "cannot upload agent binaries")
		}
	}
	return nil
}

func uploadResources(ctx context.Context, config UploadBinariesConfig) error {
	for _, res := range config.Resources {
		if res.ApplicationRevision.IsPlaceholder() {
			// Resource placeholders created in the migration import rather
			// than attempting to post empty resources.
		} else {
			err := uploadAppResource(ctx, config, res.ApplicationRevision)
			if err != nil {
				return errors.Trace(err)
			}
		}
		for unitName, unitRev := range res.UnitRevisions {
			if err := config.ResourceUploader.SetUnitResource(ctx, unitName, unitRev); err != nil {
				return errors.Annotate(err, "cannot set unit resource")
			}
		}
		// Each config.Resources element also contains a
		// CharmStoreRevision field. This isn't especially important
		// to migrate so is skipped for now.
	}
	return nil
}

func uploadAppResource(ctx context.Context, config UploadBinariesConfig, rev resources.Resource) error {
	logger.Debugf("opening application resource for %s: %s", rev.ApplicationID, rev.Name)
	reader, err := config.ResourceDownloader.OpenResource(ctx, rev.ApplicationID, rev.Name)
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
