// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/worker/secretrotate SecretManagerFacade
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/watcher SecretTriggerWatcher

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
