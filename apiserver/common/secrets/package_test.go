// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/commonsecrets_mock.go github.com/juju/juju/apiserver/common/secrets Model,Credential,SecretsConsumer,SecretsMetaState
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/core/leadership Checker,Token
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/provider_mock.go github.com/juju/juju/secrets/provider SecretBackendProvider
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state SecretsStore,SecretBackendsStorage

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
