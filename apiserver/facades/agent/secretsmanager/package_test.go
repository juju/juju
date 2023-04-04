// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsbackend.go github.com/juju/juju/apiserver/facades/agent/secretsmanager SecretsBackend
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsconsumer.go github.com/juju/juju/apiserver/facades/agent/secretsmanager SecretsConsumer
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretswatcher.go github.com/juju/juju/state StringsWatcher
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secrettriggers.go github.com/juju/juju/apiserver/facades/agent/secretsmanager SecretTriggers
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/leadershipchecker.go github.com/juju/juju/core/leadership Checker,Token
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsriggerwatcher.go github.com/juju/juju/state SecretsTriggerWatcher
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsprovider.go github.com/juju/juju/secrets/provider SecretBackendProvider

func NewTestAPI(
	authorizer facade.Authorizer,
	resources facade.Resources,
	leadership leadership.Checker,
	backend SecretsBackend,
	consumer SecretsConsumer,
	secretTriggers SecretTriggers,
	storeConfigGetter commonsecrets.BackendConfigGetter,
	providerGetter commonsecrets.ProviderInfoGetter,
	authTag names.Tag,
	clock clock.Clock,
) (*SecretsManagerAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsManagerAPI{
		authTag:           authTag,
		modelUUID:         coretesting.ModelTag.Id(),
		resources:         resources,
		leadershipChecker: leadership,
		secretsBackend:    backend,
		secretsConsumer:   consumer,
		secretsTriggers:   secretTriggers,
		storeConfigGetter: storeConfigGetter,
		providerGetter:    providerGetter,
		clock:             clock,
	}, nil
}

func (s *SecretsManagerAPI) CanManage(uri *coresecrets.URI) (leadership.Token, error) {
	return s.canManage(uri)
}
