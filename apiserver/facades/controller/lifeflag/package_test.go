// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package lifeflag_test -destination watcher_registry_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
