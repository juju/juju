// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/internal/errors"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/secret/service State,SecretBackendState,WatcherFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination provider_mock_test.go github.com/juju/juju/internal/secrets/provider SecretBackendProvider,SecretsBackend
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher,NotifyWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer

type goodToken struct{}

func (t goodToken) Check() error {
	return nil
}

type badToken struct{}

func (t badToken) Check() error {
	return errors.New("not leader")
}
