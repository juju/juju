// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package firewaller_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/controller/firewaller State,ControllerConfigAPI,ControllerConfigService,NetworkService
//go:generate go run go.uber.org/mock/mockgen -package firewaller_test -destination watcher_mock_test.go github.com/juju/juju/state NotifyWatcher
func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
