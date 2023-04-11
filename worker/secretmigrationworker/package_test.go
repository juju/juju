// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretmigrationworker

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretmigrationworker_mock.go github.com/juju/juju/worker/secretmigrationworker Logger,Facade
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secrets_mock.go github.com/juju/juju/secrets BackendsClient
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsprovider_mock.go github.com/juju/juju/secrets/provider SecretsBackend

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
