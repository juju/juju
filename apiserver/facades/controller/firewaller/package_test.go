// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package firewaller_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/controller/firewaller State,ControllerConfigAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package firewaller_test -destination watcher_mock_test.go github.com/juju/juju/state NotifyWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package firewaller_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/firewaller ControllerConfigService,ModelConfigService,NetworkService,ApplicationService,MachineService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
