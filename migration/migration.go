// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/juju/version"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.migration")

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

// UploadBackend define the methods on *state.State that are needed for
// uploading the tools and charms from the current controller to a different
// controller.
type UploadBackend interface {
	Charm(*charm.URL) (*state.Charm, error)
	ModelUUID() string
	MongoSession() *mgo.Session
	ToolsStorage() (toolstorage.StorageCloser, error)
}

// CharmUploader defines a simple single method interface that is used to
// upload a charm to the target controller
type CharmUploader interface {
	UploadCharm(*charm.URL, io.ReadSeeker) (*charm.URL, error)
}

// ToolsUploader defines a simple single method interface that is used to
// upload tools to the target controller
type ToolsUploader interface {
	UploadTools(io.ReadSeeker, version.Binary, ...string) (*tools.Tools, error)
}

// UploadBinariesConfig provides all the configuration that the UploadBinaries
// function needs to operate. The functions are configurable for testing
// purposes. To construct the config with the default functions, use
// `NewUploadBinariesConfig`.
type UploadBinariesConfig struct {
	State  UploadBackend
	Model  description.Model
	Target api.Connection

	GetCharmUploader func(api.Connection) CharmUploader
	GetToolsUploader func(api.Connection) ToolsUploader

	GetStateStorage     func(UploadBackend) storage.Storage
	GetCharmStoragePath func(UploadBackend, *charm.URL) (string, error)
}

// NewUploadBinariesConfig constructs a `UploadBinariesConfig` with the default
// functions to get the uploaders for the target api connection, and functions
// used to get the charm data out of the database.
func NewUploadBinariesConfig(backend UploadBackend, model description.Model, target api.Connection) UploadBinariesConfig {
	return UploadBinariesConfig{
		State:  backend,
		Model:  model,
		Target: target,

		GetCharmUploader:    getCharmUploader,
		GetStateStorage:     getStateStorage,
		GetToolsUploader:    getToolsUploader,
		GetCharmStoragePath: getCharmStoragePath,
	}
}

// Validate makes sure that all the config values are non-nil.
func (c *UploadBinariesConfig) Validate() error {
	if c.State == nil {
		return errors.NotValidf("missing UploadBackend")
	}
	if c.Model == nil {
		return errors.NotValidf("missing Model")
	}
	if c.Target == nil {
		return errors.NotValidf("missing Target")
	}
	if c.GetCharmUploader == nil {
		return errors.NotValidf("missing GetCharmUploader")
	}
	if c.GetStateStorage == nil {
		return errors.NotValidf("missing GetStateStorage")
	}
	if c.GetToolsUploader == nil {
		return errors.NotValidf("missing GetToolsUploader")
	}
	if c.GetCharmStoragePath == nil {
		return errors.NotValidf("missing GetCharmStoragePath")
	}
	return nil
}

// UploadBinaries will send binaries stored in the source blobstore to
// the target controller.
func UploadBinaries(config UploadBinariesConfig) error {
	if err := config.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := uploadTools(config); err != nil {
		return errors.Trace(err)
	}

	if err := uploadCharms(config); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func getStateStorage(backend UploadBackend) storage.Storage {
	return storage.NewStorage(backend.ModelUUID(), backend.MongoSession())
}

func getToolsUploader(target api.Connection) ToolsUploader {
	return target.Client()
}

func getCharmUploader(target api.Connection) CharmUploader {
	return target.Client()
}

func uploadTools(config UploadBinariesConfig) error {
	storage, err := config.State.ToolsStorage()
	if err != nil {
		return errors.Trace(err)
	}
	defer storage.Close()

	usedVersions := getUsedToolsVersions(config.Model)
	toolsUploader := config.GetToolsUploader(config.Target)

	for toolsVersion := range usedVersions {
		logger.Debugf("send tools version %s to target", toolsVersion)
		_, reader, err := storage.Tools(toolsVersion)
		if err != nil {
			return errors.Trace(err)
		}
		defer reader.Close()

		content, cleanup, err := streamThroughTempFile(reader)
		if err != nil {
			return errors.Trace(err)
		}
		defer cleanup()

		// UploadTools encapsulates the HTTP POST necessary to send the tools
		// to the target API server.
		if _, err := toolsUploader.UploadTools(content, toolsVersion); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func streamThroughTempFile(r io.Reader) (_ io.ReadSeeker, cleanup func(), err error) {
	tempFile, err := ioutil.TempFile("", "juju-tools")
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

func getUsedToolsVersions(model description.Model) map[version.Binary]bool {
	// Iterate through the model for all tools, and make a map of them.
	usedVersions := make(map[version.Binary]bool)
	// It is most likely that the preconditions will limit the number of
	// tools versions in use, but that is not depended on here.
	for _, machine := range model.Machines() {
		addToolsVersionForMachine(machine, usedVersions)
	}

	for _, service := range model.Services() {
		for _, unit := range service.Units() {
			tools := unit.Tools()
			usedVersions[tools.Version()] = true
		}
	}
	return usedVersions
}

func addToolsVersionForMachine(machine description.Machine, usedVersions map[version.Binary]bool) {
	tools := machine.Tools()
	usedVersions[tools.Version()] = true
	for _, container := range machine.Containers() {
		addToolsVersionForMachine(container, usedVersions)
	}
}

func uploadCharms(config UploadBinariesConfig) error {
	storage := config.GetStateStorage(config.State)
	usedCharms := getUsedCharms(config.Model)
	charmUploader := config.GetCharmUploader(config.Target)

	for _, charmUrl := range usedCharms.Values() {
		logger.Debugf("send charm %s to target", charmUrl)

		curl, err := charm.ParseURL(charmUrl)
		if err != nil {
			return errors.Annotate(err, "bad charm URL")
		}

		path, err := config.GetCharmStoragePath(config.State, curl)
		if err != nil {
			return errors.Trace(err)
		}

		reader, _, err := storage.Get(path)
		if err != nil {
			return errors.Annotate(err, "cannot get charm from storage")
		}
		defer reader.Close()

		content, cleanup, err := streamThroughTempFile(reader)
		if err != nil {
			return errors.Trace(err)
		}
		defer cleanup()

		if _, err := charmUploader.UploadCharm(curl, content); err != nil {
			return errors.Annotate(err, "cannot upload charm")
		}
	}
	return nil
}

func getUsedCharms(model description.Model) set.Strings {
	result := set.NewStrings()
	for _, service := range model.Services() {
		result.Add(service.CharmURL())
	}
	return result
}

func getCharmStoragePath(st UploadBackend, curl *charm.URL) (string, error) {
	ch, err := st.Charm(curl)
	if err != nil {
		return "", errors.Annotate(err, "cannot get charm from state")
	}

	return ch.StoragePath(), nil
}
