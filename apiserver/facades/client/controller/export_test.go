// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/state"
)

type patcher interface {
	PatchValue(destination, source interface{})
}

func SetPrecheckResult(p patcher, err error) {
	p.PatchValue(&runMigrationPrechecks, func(
		ctx context.Context,
		st, ctlrSt *state.State,
		agentService AgentService,
		targetInfo *coremigration.TargetInfo,
		presence facade.Presence,
		controllerConfigService ControllerConfigService,
		cloudService common.CloudService,
		credentialService common.CredentialService,
		upgradeService UpgradeService,
		_ coremodel.Model,
		modelExporter ModelExporter,
		store objectstore.ObjectStore,
		leaders map[string]string) error {
		return err
	})
}
