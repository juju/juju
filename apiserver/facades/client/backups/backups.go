// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

// Backend exposes state.State functionality needed by the backups Facade.
type Backend interface {
	IsController() bool
	Machine(id string) (Machine, error)
	MachineBase(id string) (corebase.Base, error)
	MongoSession() *mgo.Session
	ModelTag() names.ModelTag
	ModelType() state.ModelType
	ControllerTag() names.ControllerTag
	ModelConfig() (*config.Config, error)
	ControllerConfig() (controller.Config, error)
	StateServingInfo() (controller.StateServingInfo, error)
	ControllerNodes() ([]state.ControllerNode, error)
}

// API provides backup-specific API methods.
type API struct {
	backend Backend
	paths   *backups.Paths

	// machineID is the ID of the machine where the API server is running.
	machineID string
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(backend Backend, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
	err := authorizer.HasPermission(permission.SuperuserAccess, backend.ControllerTag())
	if err != nil &&
		!errors.Is(err, authentication.ErrorEntityMissingPermission) &&
		!errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	isControllerAdmin := err == nil

	if !authorizer.AuthClient() || !isControllerAdmin {
		return nil, apiservererrors.ErrPerm
	}

	// For now, backup operations are only permitted on the controller model.
	if !backend.IsController() {
		return nil, errors.New("backups are only supported from the controller model\nUse juju switch to select the controller model")
	}

	if backend.ModelType() == state.ModelTypeCAAS {
		return nil, errors.NotSupportedf("backups on kubernetes controllers")
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

	modelConfig, err := backend.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backupDir := backups.BackupDirToUse(modelConfig.BackupDir())
	paths := backups.Paths{
		BackupDir: backupDir,
		DataDir:   dataDir,
		LogsDir:   logsDir,
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

var newBackups = backups.NewBackups

// CreateResult updates the result with the information in the
// metadata value.
func CreateResult(meta *backups.Metadata, filename string) params.BackupsMetadataResult {
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
	result.Base = meta.Origin.Base

	result.ControllerUUID = meta.Controller.UUID
	result.FormatVersion = meta.FormatVersion
	result.HANodes = meta.Controller.HANodes
	result.ControllerMachineID = meta.Controller.MachineID
	result.ControllerMachineInstanceID = meta.Controller.MachineInstanceID
	result.Filename = filename

	return result
}
