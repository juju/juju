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
	paths *backups.Paths

	// machineID is the ID of the machine where the API server is running.
	machineID string
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(common.ErrPerm)
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
		st:        st,
		paths:     &paths,
		machineID: machineID,
	}
	return &b, nil
}

func extractResourceValue(resources *common.Resources, key string) (string, error) {
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

var newBackups = func(st *state.State) (backups.Backups, io.Closer) {
	stor := backups.NewStorage(st)
	return backups.NewBackups(stor), stor
}
