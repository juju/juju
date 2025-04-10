// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/modelupgrader StatePool,State,Model,UpgradeService,ControllerConfigService,ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/agents_mock.go github.com/juju/juju/apiserver/common ToolsFinder
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common BlockCheckerInterface

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
