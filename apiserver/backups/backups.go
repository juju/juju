// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

func init() {
	common.RegisterStandardFacade("Backups", 0, NewAPI)
}

var logger = loggo.GetLogger("juju.apiserver.backups")

// API serves backup-specific API methods.
type API struct {
	st      *state.State
	backups backups.Backups
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(common.ErrPerm)
	}

	stor, err := newBackupsStorage(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	b := API{
		st:      st,
		backups: backups.NewBackups(stor),
	}
	return &b, nil
}

var newBackupsStorage = func(st *state.State) (filestorage.FileStorage, error) {
	// TODO(axw,ericsnow) 2014-09-24 #1373236
	// Migrate away from legacy provider storage.
	envStor, err := environs.LegacyStorage(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	storage := state.NewBackupsStorage(st, envStor)
	return storage, nil
}
