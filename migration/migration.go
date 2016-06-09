// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.migration")

// StateExporter describes interface on state required to export a
// model.
type StateExporter interface {
	// Export generates an abstract representation of a model.
	Export() (description.Model, error)
}

// ExportModel creates a description.Model representation of the
// active model for StateExporter (typically a *state.State), and
// returns the serialized version. It provides the symmetric
// functionality to ImportModel.
func ExportModel(st StateExporter) ([]byte, error) {
	model, err := st.Export()
	if err != nil {
		return nil, errors.Trace(err)
	}
	bytes, err := description.Serialize(model)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bytes, nil
}

// ImportModel deserializes a model description from the bytes, transforms
// the model config based on information from the controller model, and then
// imports that as a new database model.
func ImportModel(st *state.State, bytes []byte) (*state.Model, *state.State, error) {
	model, err := description.Deserialize(bytes)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	controllerModel, err := st.ControllerModel()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	controllerConfig, err := controllerModel.Config()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	model.UpdateConfig(controllerValues(controllerConfig))

	if err := updateConfigFromProvider(model, controllerConfig); err != nil {
		return nil, nil, errors.Trace(err)
	}

	dbModel, dbState, err := st.Import(model)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return dbModel, dbState, nil
}

func controllerValues(config *config.Config) map[string]interface{} {
	result := make(map[string]interface{})

	result["state-port"] = config.StatePort()
	result["api-port"] = config.APIPort()
	result["controller-uuid"] = config.ControllerUUID()
	// We ignore the second bool param from the CACert check as if there
	// wasn't a CACert, there is no way we'd be importing a new model
	// into the controller
	result["ca-cert"], _ = config.CACert()
	return result
}

func updateConfigFromProvider(model description.Model, controllerConfig *config.Config) error {
	newConfig, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return errors.Trace(err)
	}

	provider, err := environs.New(newConfig)
	if err != nil {
		return errors.Trace(err)
	}

	updater, ok := provider.(environs.MigrationConfigUpdater)
	if !ok {
		return nil
	}

	model.UpdateConfig(updater.MigrationConfigUpdate(controllerConfig))
	return nil
}

// CharmDownlaoder defines a single method that is used to download a
// charm from the source controller in a migration.
type CharmDownloader interface {
	OpenCharm(*charm.URL) (io.ReadCloser, error)
}

// CharmUploader defines a single method that is used to upload a
// charm to the target controller in a migration.
type CharmUploader interface {
	UploadCharm(*charm.URL, io.ReadSeeker) (*charm.URL, error)
}

// ToolsUploader defines a single method that is used to upload tools
// to the target controller in a migration.
type ToolsUploader interface {
	UploadTools(io.ReadSeeker, version.Binary, ...string) (tools.List, error)
}

// UploadBinariesConfig provides all the configuration that the
// UploadBinaries function needs to operate. To construct the config
// with the default helper functions, use `NewUploadBinariesConfig`.
type UploadBinariesConfig struct {
	Source api.Connection
	Target api.Connection

	Charms             []string
	GetCharmDownloader func(api.Connection) CharmDownloader
	GetCharmUploader   func(api.Connection) CharmUploader

	GetToolsUploader func(api.Connection) ToolsUploader
}

// NewUploadBinariesConfig constructs a `UploadBinariesConfig` with the default
// functions to get the uploaders for the target api connection, and functions
// used to get the charm data out of the database.
func NewUploadBinariesConfig(source, target api.Connection, charms []string) UploadBinariesConfig {
	return UploadBinariesConfig{
		Source: source,
		Target: target,

		Charms:             charms,
		GetCharmDownloader: getCharmDownloader,
		GetCharmUploader:   getCharmUploader,

		GetToolsUploader: getToolsUploader,
	}
}

// Validate makes sure that all the config values are non-nil.
func (c *UploadBinariesConfig) Validate() error {
	if c.Source == nil {
		return errors.NotValidf("missing Source")
	}
	if c.Target == nil {
		return errors.NotValidf("missing Target")
	}
	if c.GetCharmDownloader == nil {
		return errors.NotValidf("missing GetCharmDownloader")
	}
	if c.GetCharmUploader == nil {
		return errors.NotValidf("missing GetCharmUploader")
	}
	if c.GetToolsUploader == nil {
		return errors.NotValidf("missing GetToolsUploader")
	}
	return nil
}

// UploadBinaries will send binaries stored in the source blobstore to
// the target controller.
func UploadBinaries(config UploadBinariesConfig) error {
	if err := config.Validate(); err != nil {
		return errors.Trace(err)
	}

	if err := uploadCharms(config); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func getCharmDownloader(source api.Connection) CharmDownloader {
	// Client is the wrong facade for a non-client to use but that's
	// where the functionality already lives and I don't have time to
	// fix the world.
	return source.Client()
}

func getCharmUploader(target api.Connection) CharmUploader {
	// Client is wrong. See above.
	return target.Client()
}

func getToolsUploader(target api.Connection) ToolsUploader {
	// Client is wrong. See above.
	return target.Client()
}

func streamThroughTempFile(r io.Reader) (_ io.ReadSeeker, cleanup func(), err error) {
	tempFile, err := ioutil.TempFile("", "juju-migrate-binary")
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			os.Remove(tempFile.Name())
		}
	}()
	_, err = io.Copy(tempFile, r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	tempFile.Seek(0, 0)
	rmTempFile := func() {
		filename := tempFile.Name()
		tempFile.Close()
		os.Remove(filename)
	}

	return tempFile, rmTempFile, nil
}

func uploadCharms(config UploadBinariesConfig) error {
	downloader := config.GetCharmDownloader(config.Source)
	uploader := config.GetCharmUploader(config.Target)

	for _, charmUrl := range config.Charms {
		logger.Debugf("send charm %s to target", charmUrl)

		curl, err := charm.ParseURL(charmUrl)
		if err != nil {
			return errors.Annotate(err, "bad charm URL")
		}

		reader, err := downloader.OpenCharm(curl)
		if err != nil {
			return errors.Annotate(err, "cannot open charm")
		}
		defer reader.Close()

		content, cleanup, err := streamThroughTempFile(reader)
		if err != nil {
			return errors.Trace(err)
		}
		defer cleanup()

		if _, err := uploader.UploadCharm(curl, content); err != nil {
			return errors.Annotate(err, "cannot upload charm")
		}
	}
	return nil
}

// PrecheckBackend is implemented by *state.State but defined as an interface
// for easier testing.
type PrecheckBackend interface {
	NeedsCleanup() (bool, error)
}

// Precheck checks the database state to make sure that the preconditions
// for model migration are met.
func Precheck(backend PrecheckBackend) error {
	cleanupNeeded, err := backend.NeedsCleanup()
	if err != nil {
		return errors.Annotate(err, "precheck cleanups")
	}
	if cleanupNeeded {
		return errors.New("precheck failed: cleanup needed")
	}
	return nil
}
