// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	corebackups "github.com/juju/juju/core/backups"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
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
	paths   *corebackups.Paths

	// machineID is the ID of the machine where the API server is running.
	machineID string
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(backend Backend, authorizer facade.Authorizer, machineTag names.Tag, dataDir, logDir string) (*API, error) {
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

	modelConfig, err := backend.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backupDir := backups.BackupDirToUse(modelConfig.BackupDir())
	paths := corebackups.Paths{
		BackupDir: backupDir,
		DataDir:   dataDir,
		LogsDir:   logDir,
	}

	b := API{
		backend:   backend,
		paths:     &paths,
		machineID: machineTag.Id(),
	}
	return &b, nil
}

var newBackups = backups.NewBackups
