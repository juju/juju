// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

var (
	NewActionAPIClient = &newAPIClient
)

func (c *DefinedCommand) ServiceTag() names.ServiceTag {
	return c.serviceTag
}

func (c *DefinedCommand) FullSchema() bool {
	return c.fullSchema
}

func (c *DoCommand) UnitTag() names.UnitTag {
	return c.unitTag
}

func (c *DoCommand) ActionName() string {
	return c.actionName
}

func (c *DoCommand) ParamsYAMLPath() string {
	return c.paramsYAML.Path
}

func (c *DoCommand) KeyValueDoArgs() [][]string {
	return c.args
}

func (c *DoCommand) ParseStrings() bool {
	return c.parseStrings
}

func ActionResultsToMap(results []params.ActionResult) map[string]interface{} {
	return resultsToMap(results)
}
