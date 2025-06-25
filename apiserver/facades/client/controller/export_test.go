// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/logger"
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
		logger logger.Logger,
		st, ctlrSt *state.State,
		targetInfo *coremigration.TargetInfo,
		controllerConfigService ControllerConfigService,
		credentialServiceGetter func(context.Context, coremodel.UUID) (CredentialService, error),
		modelAgentServiceGetter func(context.Context, coremodel.UUID) (ModelAgentService, error),
		modelConfigService ModelConfigService,
		upgradeServiceGetter func(context.Context, coremodel.UUID) (UpgradeService, error),
		modelService ModelService,
		applicationServiceGetter func(context.Context, coremodel.UUID) (ApplicationService, error),
		relationServiceGetter func(context.Context, coremodel.UUID) (RelationService, error),
		statusServiceGetter func(context.Context, coremodel.UUID) (StatusService, error),
		modelExporter func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (ModelExporter, error),
		store objectstore.ObjectStore,
		model coremodel.Model,
		controllerModelUUID coremodel.UUID,
	) error {
		return err
	})
}
