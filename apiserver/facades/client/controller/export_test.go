// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/state"
)

type patcher interface {
	PatchValue(destination, source interface{})
}

func SetPrecheckResult(p patcher, err error) {
	p.PatchValue(&runMigrationPrechecks, func(ctx context.Context,
		st, ctlrSt *state.State,
		targetInfo *coremigration.TargetInfo,
		presence facade.Presence,
		controllerConfigService ControllerConfigService,
		cloudService common.CloudService,
		credentialService common.CredentialService,
		upgradeService UpgradeService,
		modelService ModelService,
		modelExporter ModelExporter,
		store objectstore.ObjectStore,
		leaders map[string]string) error {
		return err
	})
}
