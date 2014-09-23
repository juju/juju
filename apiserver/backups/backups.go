// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/files"
)

func init() {
	common.RegisterStandardFacade("Backups", 0, NewAPI)
}

var logger = loggo.GetLogger("juju.apiserver.backups")

// API serves backup-specific API methods.
type API struct {
	st      *state.State
	paths   files.Paths
	backups backups.Backups
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

	var paths files.Paths
	paths.DataDir = dataDir.String()
	paths.LogsDir = logDir.String()

	backups, err := NewBackups(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	b := API{
		st:      st,
		paths:   paths,
		backups: backups,
	}
	return &b, nil
}

// NewBackups returns a new Backups based on the given state.
func NewBackups(st *state.State) (backups.Backups, error) {
	stor, err := newBackupsStorage(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return backups.NewBackups(stor), nil
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

// PublicAddress implements the server side of Client.PublicAddress.
func (a *API) PublicAddress(p params.PublicAddress) (results params.PublicAddressResults, err error) {
	switch {
	case names.IsValidMachine(p.Target):
		machine, err := a.st.Machine(p.Target)
		if err != nil {
			return results, err
		}
		addr := network.SelectPublicAddress(machine.Addresses())
		if addr == "" {
			return results, errors.Errorf("machine %q has no public address", machine)
		}
		return params.PublicAddressResults{PublicAddress: addr}, nil

	case names.IsValidUnit(p.Target):
		unit, err := a.st.Unit(p.Target)
		if err != nil {
			return results, err
		}
		addr, ok := unit.PublicAddress()
		if !ok {
			return results, errors.Errorf("unit %q has no public address", unit)
		}
		return params.PublicAddressResults{PublicAddress: addr}, nil
	}
	return results, errors.Errorf("unknown unit or machine %q", p.Target)
}
