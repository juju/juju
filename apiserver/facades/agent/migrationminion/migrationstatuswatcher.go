// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coremigration "github.com/juju/juju/core/migration"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

func NewMigrationStatusWatcherAPI(
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	modelMigrationService ModelMigrationService,
	controllerNodeService ControllerNodeService,
	controllerConfigService ControllerConfigService,
	id string,
	dispose func(),
) (*MigrationStatusWatcherAPI, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent()) {
		return nil, apiservererrors.ErrPerm
	}
	w, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, ok := w.(corewatcher.NotifyWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	stop := func() error {
		dispose()
		return watcherRegistry.Stop(id)
	}
	return &MigrationStatusWatcherAPI{
		stop:                    stop,
		watcher:                 watcher,
		modelMigrationService:   modelMigrationService,
		controllerNodeService:   controllerNodeService,
		controllerConfigService: controllerConfigService,
	}, nil
}

type MigrationStatusWatcherAPI struct {
	stop    func() error
	watcher corewatcher.NotifyWatcher

	modelMigrationService   ModelMigrationService
	controllerNodeService   ControllerNodeService
	controllerConfigService ControllerConfigService
}

// Stop stops the watcher.
func (w *MigrationStatusWatcherAPI) Stop() error {
	return w.stop()
}

// Next returns when the status for a model migration for the
// associated model changes. The current details for the active
// migration are returned.
func (w *MigrationStatusWatcherAPI) Next(ctx context.Context) (params.MigrationStatus, error) {
	_, err := internal.FirstResult(ctx, w.watcher)
	if err != nil {
		return params.MigrationStatus{}, errors.Trace(err)
	}

	mig, err := w.modelMigrationService.Migration(ctx)
	if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "migration lookup")
	}
	if mig.Phase == coremigration.NONE {
		return params.MigrationStatus{
			Phase: coremigration.NONE.String(),
		}, nil
	}

	sourceAddrs, err := w.controllerNodeService.GetAllAPIAddressesForClients(ctx)
	if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "retrieving source addresses")
	}

	cfg, err := w.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.MigrationStatus{}, errors.Annotate(err, "retrieving controller config")
	}
	sourceCACert, ok := cfg.CACert()
	if !ok {
		return params.MigrationStatus{}, errors.New("missing CA cert for controller model")
	}

	return params.MigrationStatus{
		MigrationId:    mig.UUID,
		Attempt:        mig.Attempt,
		Phase:          mig.Phase.String(),
		SourceAPIAddrs: sourceAddrs,
		SourceCACert:   sourceCACert,
		TargetAPIAddrs: mig.Target.Addrs,
		TargetCACert:   mig.Target.CACert,
	}, nil
}
