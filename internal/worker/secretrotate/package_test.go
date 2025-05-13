// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/client_mock.go -source secretrotate.go
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/watcher SecretTriggerWatcher

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
