// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
)

type patcher interface {
	PatchValue(destination, source interface{})
}

func SetPrecheckResult(p patcher, err error) {
	p.PatchValue(&runMigrationPrechecks, func(*state.State, migration.TargetInfo) error {
		return err
	})
}
