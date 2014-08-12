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
	common.RegisterStandardFacade("Backups", 0, NewBackupsAPI)
}

var logger = loggo.GetLogger("juju.state.apiserver.backups")

// Backups serves backup-specific API methods.
type BackupsAPI struct {
	st      *state.State
	backups backups.Backups
}

// NewBackups creates a new instance of the Backups Facade.
func NewBackupsAPI(
	st *state.State, resources *common.Resources, authorizer common.Authorizer,
) (*BackupsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	stor, err := newBackupsStorage(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	b := BackupsAPI{
		st:      st,
		backups: backups.NewBackups(stor),
	}
	return &b, nil
}

var newBackupsStorage = func(st *state.State) (filestorage.FileStorage, error) {
	envStor, err := environs.GetStorage(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	storage := state.NewBackupsStorage(st, envStor)
	return storage, nil
}
