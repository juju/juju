// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/commonsecrets_mock.go github.com/juju/juju/apiserver/common/secrets ModelConfig,SecretService,SecretBackendService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/authorizer_mock.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/core/leadership Checker,Token
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/statewatcher.go github.com/juju/juju/state NotifyWatcher

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var (
	NewSecretBackendModelConfigWatcher = newSecretBackendModelConfigWatcher
)
