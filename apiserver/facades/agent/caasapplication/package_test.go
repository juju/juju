// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

// TODO - only needed until controller state is removed.
//go:generate go run go.uber.org/mock/mockgen -typed -package caasapplication -destination state_mock_test.go github.com/juju/juju/apiserver/facades/agent/caasapplication ControllerState

//go:generate go run go.uber.org/mock/mockgen -typed -package caasapplication -destination package_mock_test.go github.com/juju/juju/apiserver/facades/agent/caasapplication ControllerConfigService,ApplicationService,ModelAgentService

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}
