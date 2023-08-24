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

// //go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretswatcher.go github.com/juju/juju/state StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/backendstate.go github.com/juju/juju/apiserver/facades/controller/secretbackendmanager BackendState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/backendrotate.go github.com/juju/juju/apiserver/facades/controller/secretbackendmanager BackendRotate
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/backendrotateatcher.go github.com/juju/juju/state SecretBackendRotateWatcher
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsprovider.go github.com/juju/juju/internal/secrets/provider SecretBackendProvider

func NewTestAPI(
	authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	secretsState BackendState,
	backendrotate BackendRotate,
	clock clock.Clock,
) (*SecretBackendsManagerAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretBackendsManagerAPI{
		watcherRegistry: watcherRegistry,
		backendState:    secretsState,
		backendRotate:   backendrotate,
		clock:           clock,
	}, nil
}
