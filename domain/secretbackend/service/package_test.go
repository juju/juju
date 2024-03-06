// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package service -destination state_mock_test.go github.com/juju/juju/domain/secretbackend/service State
//go:generate go run go.uber.org/mock/mockgen -package service -destination watcher_mock_test.go github.com/juju/juju/domain/secretbackend WatcherFactory,SecretBackendRotateWatcher
//go:generate go run go.uber.org/mock/mockgen -package service -destination provider_mock_test.go github.com/juju/juju/internal/secrets/provider SecretBackendProvider
//go:generate go run go.uber.org/mock/mockgen -package service -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -package service -destination secrets_provider_mock_test.go github.com/juju/juju/internal/secrets/provider SecretsBackend
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
