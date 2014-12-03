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
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(common.ErrPerm)
	}

	// Get the backup paths.
	values, err := extractResourceValues(resources, "dataDir", "logDir")
	if err != nil {
		return nil, errors.Trace(err)
	}
	paths := backups.Paths{
		DataDir: values["dataDir"],
		LogsDir: values["logDir"],
	}

	// Build the API.
	b := API{
		st:    st,
		paths: &paths,
	}
	return &b, nil
}

func extractResourceValues(resources *common.Resources, keys ...string) (map[string]string, error) {
	resourceValues := make(map[string]string)
	for _, key := range keys {
		res := resources.Get(key)
		strRes, ok := res.(common.StringResource)
		if !ok {
			if res == nil {
				strRes = ""
			} else {
				return nil, errors.Errorf("invalid %s resource: %v", key, res)
			}
		}
		resourceValues[key] = strRes.String()
	}
	return resourceValues, nil
}

var newBackups = func(st *state.State) (backups.Backups, io.Closer) {
	stor := backups.NewStorage(st)
	return backups.NewBackups(stor), stor
}
