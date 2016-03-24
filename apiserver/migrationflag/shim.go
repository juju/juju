// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("MigrationFlag", 1, newFacade)
}

func newFacade(st *state.State, resources *common.Resources, auth common.Authorizer) (*Facade, error) {
	facade, err := New(&backend{st}, resources, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

type backend struct {
	st *state.State
}

func (shim *backend) ModelUUID() string {
	return shim.st.ModelUUID()
}

func (shim *backend) WatchMigrationPhase() (state.NotifyWatcher, error) {
	watcher, err := shim.st.WatchMigrationStatus()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return watcher, nil
}

func (shim *backend) MigrationPhase() (migration.Phase, error) {
	mig, err := shim.st.GetModelMigration()
	if errors.IsNotFound(err) {
		return migration.NONE, nil
	} else if err != nil {
		return migration.UNKNOWN, errors.Trace(err)
	}
	phase, err := mig.Phase()
	if err != nil {
		return migration.UNKNOWN, errors.Trace(err)
	}
	return phase, nil
}
