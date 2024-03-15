// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package state -destination watcherfactory_mock_test.go github.com/juju/juju/domain/secretbackend WatcherFactory
//go:generate go run go.uber.org/mock/mockgen -package state -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
