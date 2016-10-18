// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("MigrationFlag", 1, newFacade)
}

// newFacade wraps New to express the supplied *state.State as a Backend.
func newFacade(st *state.State, resources facade.Resources, auth facade.Authorizer) (*Facade, error) {
	facade, err := New(&backend{st}, resources, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

// backend implements Backend by wrapping a *state.State.
type backend struct {
	st *state.State
}

// ModelUUID is part of the Backend interface.
func (shim *backend) ModelUUID() string {
	return shim.st.ModelUUID()
}

// WatchMigrationPhase is part of the Backend interface.
func (shim *backend) WatchMigrationPhase() state.NotifyWatcher {
	return shim.st.WatchMigrationStatus()
}

// MigrationPhase is part of the Backend interface.
func (shim *backend) MigrationPhase() (migration.Phase, error) {
	mig, err := shim.st.LatestMigration()
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
