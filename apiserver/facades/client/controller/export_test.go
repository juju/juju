// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

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
	p.PatchValue(&runMigrationPrechecks, func(ctx context.Context,
		st, ctlrSt *state.State,
		targetInfo *coremigration.TargetInfo,
		controllerConfigService ControllerConfigService,
		credentialService CredentialService,
		modelAgentService ModelAgentService,
		modelConfigService ModelConfigService,
		upgradeService UpgradeService,
		modelService ModelService,
		applicationService ApplicationService,
		statusService StatusService,
		modelExporter func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (ModelExporter, error),
		store objectstore.ObjectStore,
		model coremodel.Model) error {
		return err
	})
}
