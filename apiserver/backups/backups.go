// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

var logger = loggo.GetLogger("juju.apiserver.backups")

// Backend exposes state.State functionality needed by the backups Facade.
type Backend interface {
	IsController() bool
	Machine(id string) (*state.Machine, error)
	MachineSeries(id string) (string, error)
	MongoConnectionInfo() *mongo.MongoInfo
	MongoSession() *mgo.Session
	ModelTag() names.ModelTag
	ControllerTag() names.ControllerTag
	ModelConfig() (*config.Config, error)
	ControllerConfig() (controller.Config, error)
	StateServingInfo() (state.StateServingInfo, error)
	RestoreInfo() *state.RestoreInfo
}

// API serves backup-specific API methods.
type API struct {
	backend Backend
	paths   *backups.Paths

	// machineID is the ID of the machine where the API server is running.
	machineID string
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(backend Backend, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
	isControllerAdmin, err := authorizer.HasPermission(description.SuperuserAccess, backend.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	if !authorizer.AuthClient() || !isControllerAdmin {
		return nil, common.ErrPerm
	}

	// For now, backup operations are only permitted on the controller environment.
	if !backend.IsController() {
		return nil, errors.New("backups are not supported for hosted models")
	}

	// Get the backup paths.
	dataDir, err := extractResourceValue(resources, "dataDir")
	if err != nil {
		return nil, errors.Trace(err)
	}
	logsDir, err := extractResourceValue(resources, "logDir")
	if err != nil {
		return nil, errors.Trace(err)
	}
	paths := backups.Paths{
		DataDir: dataDir,
		LogsDir: logsDir,
	}

	// Build the API.
	machineID, err := extractResourceValue(resources, "machineID")
	if err != nil {
		return nil, errors.Trace(err)
	}
	b := API{
		backend:   backend,
		paths:     &paths,
		machineID: machineID,
	}
	return &b, nil
}

func extractResourceValue(resources facade.Resources, key string) (string, error) {
	res := resources.Get(key)
	strRes, ok := res.(common.StringResource)
	if !ok {
		if res == nil {
			strRes = ""
		} else {
			return "", errors.Errorf("invalid %s resource: %v", key, res)
		}
	}
	return strRes.String(), nil
}

var newBackups = func(backend Backend) (backups.Backups, io.Closer) {
	stor := backups.NewStorage(backend)
	return backups.NewBackups(stor), stor
}

// ResultFromMetadata updates the result with the information in the
// metadata value.
func ResultFromMetadata(meta *backups.Metadata) params.BackupsMetadataResult {
	var result params.BackupsMetadataResult

	result.ID = meta.ID()

	result.Checksum = meta.Checksum()
	result.ChecksumFormat = meta.ChecksumFormat()
	result.Size = meta.Size()
	if meta.Stored() != nil {
		result.Stored = *(meta.Stored())
	}

	result.Started = meta.Started
	if meta.Finished != nil {
		result.Finished = *meta.Finished
	}
	result.Notes = meta.Notes

	result.Model = meta.Origin.Model
	result.Machine = meta.Origin.Machine
	result.Hostname = meta.Origin.Hostname
	result.Version = meta.Origin.Version
	result.Series = meta.Origin.Series

	// TODO(wallyworld) - remove these ASAP
	// These are only used by the restore CLI when re-bootstrapping.
	// We will use a better solution but the way restore currently
	// works, we need them and they are no longer available via
	// bootstrap config. We will need to ifx how re-bootstrap deals
	// with these keys to address the issue.
	result.CACert = meta.CACert
	result.CAPrivateKey = meta.CAPrivateKey

	return result
}

// MetadataFromResult returns a new Metadata based on the result. The ID
// of the metadata is not set. Call meta.SetID() if that is desired.
// Likewise with Stored and meta.SetStored().
func MetadataFromResult(result params.BackupsMetadataResult) *backups.Metadata {
	meta := backups.NewMetadata()
	meta.Started = result.Started
	if !result.Finished.IsZero() {
		meta.Finished = &result.Finished
	}
	meta.Origin.Model = result.Model
	meta.Origin.Machine = result.Machine
	meta.Origin.Hostname = result.Hostname
	meta.Origin.Version = result.Version
	meta.Origin.Series = result.Series
	meta.Notes = result.Notes
	meta.SetFileInfo(result.Size, result.Checksum, result.ChecksumFormat)
	return meta
}
