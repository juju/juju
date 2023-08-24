// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"testing"

	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsstate.go github.com/juju/juju/apiserver/facades/agent/secretsdrain SecretsState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsconsumer.go github.com/juju/juju/apiserver/facades/agent/secretsdrain SecretsConsumer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/modelstate.go github.com/juju/juju/apiserver/facades/agent/secretsdrain Model
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/statewatcher.go github.com/juju/juju/state NotifyWatcher
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadershipchecker.go github.com/juju/juju/core/leadership Checker,Token
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsprovider.go github.com/juju/juju/internal/secrets/provider SecretBackendProvider

func NewTestAPI(
	authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	leadership leadership.Checker,
	secretsState SecretsState,
	model Model,
	consumer SecretsConsumer,
	authTag names.Tag,
) (*SecretsDrainAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsDrainAPI{
		authTag:           authTag,
		watcherRegistry:   watcherRegistry,
		leadershipChecker: leadership,
		secretsState:      secretsState,
		model:             model,
		secretsConsumer:   consumer,
	}, nil
}

var (
	NewSecretBackendModelConfigWatcher = newSecretBackendModelConfigWatcher
)
