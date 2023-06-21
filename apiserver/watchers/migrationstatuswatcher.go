// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var getMigrationBackend = func(st *state.State) migrationBackend {
	return st
}

var getControllerBackend = func(pool *state.StatePool) (controllerBackend, error) {
	return pool.SystemState()
}

// migrationBackend defines model State functionality required by the
// migration watchers.
type migrationBackend interface {
	LatestMigration() (state.ModelMigration, error)
}

// migrationBackend defines controller State functionality required by the
// migration watchers.
type controllerBackend interface {
	APIHostPortsForClients() ([]network.SpaceHostPorts, error)
	ControllerConfig() (controller.Config, error)
}

func NewMigrationStatusWatcher(context facade.Context) (facade.Facade, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
		resources       = context.Resources()
	)

	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, err := GetWatcherByID(watcherRegistry, resources, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	notifyWatcher, ok := watcher.(state.NotifyWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}

	controllerBackend, err := getControllerBackend(context.StatePool())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &srvMigrationStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       notifyWatcher,
		st:            getMigrationBackend(context.State()),
		ctrlSt:        controllerBackend,
	}, nil
}

type srvMigrationStatusWatcher struct {
	watcherCommon
	watcher state.NotifyWatcher
	st      migrationBackend
	ctrlSt  controllerBackend
}

// Next returns when the status for a model migration for the
// associated model changes. The current details for the active
// migration are returned.
func (w *srvMigrationStatusWatcher) Next() (params.MigrationStatus, error) {
	empty := params.MigrationStatus{}

	if _, ok := <-w.watcher.Changes(); !ok {
		err := w.watcher.Err()
		if err == nil {
			err = apiservererrors.ErrStoppedWatcher
		}
		return empty, err
	}

	mig, err := w.st.LatestMigration()
	if errors.IsNotFound(err) {
		return params.MigrationStatus{
			Phase: migration.NONE.String(),
		}, nil
	} else if err != nil {
		return empty, errors.Annotate(err, "migration lookup")
	}

	phase, err := mig.Phase()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving migration phase")
	}

	sourceAddrs, err := w.getLocalHostPorts()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving source addresses")
	}

	sourceCACert, err := getControllerCACert(w.ctrlSt)
	if err != nil {
		return empty, errors.Annotate(err, "retrieving source CA cert")
	}

	target, err := mig.TargetInfo()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving target info")
	}

	return params.MigrationStatus{
		MigrationId:    mig.Id(),
		Attempt:        mig.Attempt(),
		Phase:          phase.String(),
		SourceAPIAddrs: sourceAddrs,
		SourceCACert:   sourceCACert,
		TargetAPIAddrs: target.Addrs,
		TargetCACert:   target.CACert,
	}, nil
}

func (w *srvMigrationStatusWatcher) getLocalHostPorts() ([]string, error) {
	hostports, err := w.ctrlSt.APIHostPortsForClients()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var out []string
	for _, section := range hostports {
		for _, hostport := range section {
			out = append(out, hostport.String())
		}
	}
	return out, nil
}

// This is a shim to avoid the need to use a working State into the
// unit tests. It is tested as part of the client side API tests.
var getControllerCACert = func(st controllerBackend) (string, error) {
	cfg, err := st.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}

	cacert, ok := cfg.CACert()
	if !ok {
		return "", errors.New("missing CA cert for controller model")
	}
	return cacert, nil
}
