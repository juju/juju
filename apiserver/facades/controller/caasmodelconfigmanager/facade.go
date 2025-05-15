// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// ControllerConfigService provides required controller config watcher for the Facade.
type ControllerConfigService interface {
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
type Facade struct {
	auth            facade.Authorizer
	watcherRegistry facade.WatcherRegistry

	controllerConfigAPI *common.ControllerConfigAPI

	controllerConfigService ControllerConfigService
}

func (f *Facade) ControllerConfig(ctx context.Context) (params.ControllerConfigResult, error) {
	return f.controllerConfigAPI.ControllerConfig(ctx)
}

func (f *Facade) WatchControllerConfig(ctx context.Context) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	w, err := f.controllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.StringsWatcherId, result.Changes, err = internal.EnsureRegisterWatcher[[]string](ctx, f.watcherRegistry, w)
	result.Error = apiservererrors.ServerError(err)
	return result, nil
}
