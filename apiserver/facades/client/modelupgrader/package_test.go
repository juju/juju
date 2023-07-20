// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	stdtesting "testing"

	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/modelupgrader StatePool,State,Model
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/agents_mock.go github.com/juju/juju/apiserver/common ToolsFinder
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs BootstrapEnviron
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common BlockCheckerInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/facades/client/modelupgrader ControllerConfigGetter

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func (m *ModelUpgraderAPI) FindAgents(args common.FindAgentsParams) (coretools.Versions, error) {
	return m.findAgents(args)
}

func (m *ModelUpgraderAPI) DecideVersion(
	currentVersion version.Number, args common.FindAgentsParams,
) (_ version.Number, err error) {
	return m.decideVersion(currentVersion, args)
}
