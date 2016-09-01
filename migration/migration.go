// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"io"
	"io/ioutil"
	"net/url"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/core/description"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
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

	dbModel, dbState, err := st.Import(model)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return dbModel, dbState, nil
}

// CharmDownlaoder defines a single method that is used to download a
// charm from the source controller in a migration.
type CharmDownloader interface {
	OpenCharm(*charm.URL) (io.ReadCloser, error)
}

// UploadBackend define the methods on *state.State that are needed for
// uploading the tools and charms from the current controller to a different
// controller.
type UploadBackend interface {
	Charm(*charm.URL) (*state.Charm, error)
	ModelUUID() string
	MongoSession() *mgo.Session
	ToolsStorage() (binarystorage.StorageCloser, error)
}

// CharmUploader defines a single method that is used to upload a
// charm to the target controller in a migration.
type CharmUploader interface {
	UploadCharm(*charm.URL, io.ReadSeeker) (*charm.URL, error)
}

// ToolsDownloader defines a single method that is used to download
// tools from the source controller in a migration.
type ToolsDownloader interface {
	OpenURI(string, url.Values) (io.ReadCloser, error)
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
	Charms          []string
	CharmDownloader CharmDownloader
	CharmUploader   CharmUploader

	Tools           map[version.Binary]string
	ToolsDownloader ToolsDownloader
	ToolsUploader   ToolsUploader
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
	if err := uploadTools(config); err != nil {
		return errors.Trace(err)
	}
	return nil
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
	for _, charmUrl := range config.Charms {
		logger.Debugf("sending charm %s to target", charmUrl)

		curl, err := charm.ParseURL(charmUrl)
		if err != nil {
			return errors.Annotate(err, "bad charm URL")
		}

		reader, err := config.CharmDownloader.OpenCharm(curl)
		if err != nil {
			return errors.Annotate(err, "cannot open charm")
		}
		defer reader.Close()

		content, cleanup, err := streamThroughTempFile(reader)
		if err != nil {
			return errors.Trace(err)
		}
		defer cleanup()

		if _, err := config.CharmUploader.UploadCharm(curl, content); err != nil {
			return errors.Annotate(err, "cannot upload charm")
		}
	}
	return nil
}

func uploadTools(config UploadBinariesConfig) error {
	for v, uri := range config.Tools {
		logger.Debugf("sending tools to target: %s", v)

		reader, err := config.ToolsDownloader.OpenURI(uri, nil)
		if err != nil {
			return errors.Annotate(err, "cannot open charm")
		}
		defer reader.Close()

		content, cleanup, err := streamThroughTempFile(reader)
		if err != nil {
			return errors.Trace(err)
		}
		defer cleanup()

		if _, err := config.ToolsUploader.UploadTools(content, v); err != nil {
			return errors.Annotate(err, "cannot upload tools")
		}
	}
	return nil
}
