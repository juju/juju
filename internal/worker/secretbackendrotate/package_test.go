// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/internal/worker/secretbackendrotate SecretBackendManagerFacade
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/watcher SecretBackendRotateWatcher
