// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"github.com/juju/clock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package secretbackendmanager -destination mock_service.go -source service.go BackendService
//go:generate go run go.uber.org/mock/mockgen -typed -package secretbackendmanager -destination mock_watcher.go github.com/juju/juju/core/watcher SecretBackendRotateWatcher

func NewTestAPI(
	authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	backendService BackendService,
	clock clock.Clock,
) (*SecretBackendsManagerAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretBackendsManagerAPI{
		watcherRegistry: watcherRegistry,
		backendService:  backendService,
		clock:           clock,
	}, nil
}
