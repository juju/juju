// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
)

type patcher interface {
	PatchValue(destination, source interface{})
}

func SetPrecheckResult(p patcher, err error) {
	p.PatchValue(&runMigrationPrechecks, func(context.Context, *state.State, *state.State, *migration.TargetInfo, facade.Presence, ControllerConfigGetter, common.CredentialService) error {
		return err
	})
}

var (
	NewControllerAPIv11 = newControllerAPIv11
)
