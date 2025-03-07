// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsdrainworker_mock.go github.com/juju/juju/internal/worker/secretsdrainworker Logger,SecretsDrainFacade
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secrets_mock.go github.com/juju/juju/secrets BackendsClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsprovider_mock.go github.com/juju/juju/secrets/provider SecretsBackend
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/core/leadership TrackerWorker

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
