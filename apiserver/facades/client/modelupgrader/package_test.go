// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	stdtesting "testing"

	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/modelupgrader StatePool,State,Model,UpgradeService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/agents_mock.go github.com/juju/juju/apiserver/common ToolsFinder
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs BootstrapEnviron
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common BlockCheckerInterface

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func (m *ModelUpgraderAPI) FindAgents(ctx context.Context, args common.FindAgentsParams) (coretools.Versions, error) {
	return m.findAgents(ctx, args)
}

func (m *ModelUpgraderAPI) DecideVersion(
	ctx context.Context,
	currentVersion version.Number, args common.FindAgentsParams,
) (_ version.Number, err error) {
	return m.decideVersion(ctx, currentVersion, args)
}
