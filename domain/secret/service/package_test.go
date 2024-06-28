// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination state_mock_test.go github.com/juju/juju/domain/secret/service State
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination provider_mock_test.go github.com/juju/juju/internal/secrets/provider SecretBackendProvider,SecretsBackend
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination watcherfactory_mock_test.go github.com/juju/juju/domain/secret/service WatcherFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher,NotifyWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination token_mock_test.go github.com/juju/juju/core/leadership Token

func Test(t *testing.T) {
	gc.TestingT(t)
}
