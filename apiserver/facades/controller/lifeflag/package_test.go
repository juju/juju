// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package lifeflag_test -destination watcher_registry_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
