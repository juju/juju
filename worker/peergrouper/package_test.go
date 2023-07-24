// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/changestream_mock.go github.com/juju/juju/core/changestream WatchableDBGetter
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/worker/peergrouper ControllerConfigService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/watcher_mock.go -source=./../../core/watcher/watcher.go

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
