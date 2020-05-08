// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/names/v4"
)

var (
	GetNewRunnerExecutor = getNewRunnerExecutor
	JujudSymlinks        = jujudSymlinks
)

type (
	CaasOperator = caasOperator
)

func (op *caasOperator) MakeAgentSymlinks(unitTag names.UnitTag) error {
	return op.makeAgentSymlinks(unitTag)
}

func (op *caasOperator) GetDataDir() string {
	return op.config.DataDir
}

func (op *caasOperator) GetToolsDir() string {
	return op.paths.GetToolsDir()
}
