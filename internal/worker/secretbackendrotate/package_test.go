// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/client_mock.go -source rotate.go
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/watcher SecretBackendRotateWatcher

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
