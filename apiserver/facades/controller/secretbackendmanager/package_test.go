// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"testing"

	"github.com/juju/clock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -package secretbackendmanager -destination mock_service.go -source service.go BackendService
//go:generate go run go.uber.org/mock/mockgen -package secretbackendmanager -destination mock_watcher.go github.com/juju/juju/core/watcher SecretBackendRotateWatcher

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
