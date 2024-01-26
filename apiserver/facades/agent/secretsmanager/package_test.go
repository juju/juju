// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsstate.go github.com/juju/juju/apiserver/facades/agent/secretsmanager SecretsState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsconsumer.go github.com/juju/juju/apiserver/facades/agent/secretsmanager SecretsConsumer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/crossmodel.go github.com/juju/juju/apiserver/facades/agent/secretsmanager CrossModelState,CrossModelSecretsClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretswatcher.go github.com/juju/juju/state StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secrettriggers.go github.com/juju/juju/apiserver/facades/agent/secretsmanager SecretTriggers
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadershipchecker.go github.com/juju/juju/core/leadership Checker,Token
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsriggerwatcher.go github.com/juju/juju/state SecretsTriggerWatcher
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsprovider.go github.com/juju/juju/internal/secrets/provider SecretBackendProvider

func NewTestAPI(
	authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	leadership leadership.Checker,
	secretsState SecretsState,
	consumer SecretsConsumer,
	secretTriggers SecretTriggers,
	backendConfigGetter commonsecrets.BackendConfigGetter,
	adminConfigGetter commonsecrets.BackendAdminConfigGetter,
	drainConfigGetter commonsecrets.BackendDrainConfigGetter,
	remoteClientGetter func(ctx context.Context, uri *coresecrets.URI) (CrossModelSecretsClient, error),
	crossModelState CrossModelState,
	authTag names.Tag,
	clock clock.Clock,
) (*SecretsManagerAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsManagerAPI{
		authTag:             authTag,
		watcherRegistry:     watcherRegistry,
		authorizer:          authorizer,
		leadershipChecker:   leadership,
		secretsState:        secretsState,
		secretsConsumer:     consumer,
		secretsTriggers:     secretTriggers,
		backendConfigGetter: backendConfigGetter,
		adminConfigGetter:   adminConfigGetter,
		drainConfigGetter:   drainConfigGetter,
		remoteClientGetter:  remoteClientGetter,
		crossModelState:     crossModelState,
		clock:               clock,
		controllerUUID:      coretesting.ControllerTag.Id(),
		modelUUID:           coretesting.ModelTag.Id(),
		logger:              loggo.GetLoggerWithLabels("juju.apiserver.secretsmanager", corelogger.SECRETS),
	}, nil
}
