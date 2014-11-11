// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

func init() {
	common.RegisterStandardFacade("Backups", 0, NewAPI)
}

var logger = loggo.GetLogger("juju.apiserver.backups")

// API serves backup-specific API methods.
type API struct {
	st    *state.State
	paths backups.Paths
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(common.ErrPerm)
	}

	dataDirRes := resources.Get("dataDir")
	dataDir, ok := dataDirRes.(common.StringResource)
	if !ok {
		if dataDirRes == nil {
			dataDir = ""
		} else {
			return nil, errors.Errorf("invalid dataDir resource: %v", dataDirRes)
		}
	}

	logDirRes := resources.Get("logDir")
	logDir, ok := logDirRes.(common.StringResource)
	if !ok {
		if logDirRes == nil {
			logDir = ""
		} else {
			return nil, errors.Errorf("invalid logDir resource: %v", logDirRes)
		}
	}

	var paths backups.Paths
	paths.DataDir = dataDir.String()
	paths.LogsDir = logDir.String()

	b := API{
		st:    st,
		paths: paths,
	}
	return &b, nil
}

var newBackups = func(st *state.State) (backups.Backups, io.Closer) {
	backups, stor := state.NewBackups(st)
	return backups, stor
}
