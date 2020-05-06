// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/names/v4"
)

var GetNewRunnerExecutor = getNewRunnerExecutor

func (op *caasOperator) MakeAgentSymlinks(unitTag names.UnitTag) error {
	return op.makeAgentSymlinks(unitTag)
}
