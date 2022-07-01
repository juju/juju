// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/core/migration"
	"github.com/juju/juju/v2/state"
)

type patcher interface {
	PatchValue(destination, source interface{})
}

func SetPrecheckResult(p patcher, err error) {
	p.PatchValue(&runMigrationPrechecks, func(*state.State, *state.State, *migration.TargetInfo, facade.Presence) error {
		return err
	})
}

var (
	NewControllerAPIv3  = newControllerAPIv3
	NewControllerAPIv4  = newControllerAPIv4
	NewControllerAPIv5  = newControllerAPIv5
	NewControllerAPIv11 = newControllerAPIv11
)
