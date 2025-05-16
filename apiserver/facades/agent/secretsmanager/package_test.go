// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/secrets.go -source service.go
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/secretsriggerwatcher.go github.com/juju/juju/core/watcher SecretTriggerWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/crossmodel.go github.com/juju/juju/apiserver/facades/agent/secretsmanager CrossModelState,CrossModelSecretsClient
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/secretswatcher.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/leadershipchecker.go github.com/juju/juju/core/leadership Checker,Token

func NewTestAPI(
	c *tc.C,
	authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	leadership leadership.Checker,
	secretService SecretService,
	consumer SecretsConsumer,
	secretTriggers SecretTriggers,
	secretBackendService SecretBackendService,
	remoteClientGetter func(ctx context.Context, uri *coresecrets.URI) (CrossModelSecretsClient, error),
	crossModelState CrossModelState,
	authTag names.Tag,
	clock clock.Clock,
) (*SecretsManagerAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsManagerAPI{
		authTag:              authTag,
		watcherRegistry:      watcherRegistry,
		authorizer:           authorizer,
		leadershipChecker:    leadership,
		secretBackendService: secretBackendService,
		secretService:        secretService,
		secretsConsumer:      consumer,
		secretsTriggers:      secretTriggers,
		remoteClientGetter:   remoteClientGetter,
		crossModelState:      crossModelState,
		clock:                clock,
		controllerUUID:       coretesting.ControllerTag.Id(),
		modelUUID:            coretesting.ModelTag.Id(),
		logger:               loggertesting.WrapCheckLog(c),
	}, nil
}
