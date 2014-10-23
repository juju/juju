// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/names"

var (
	NewActionAPIClient = &newAPIClient
)

func (c *DefinedCommand) ServiceTag() names.ServiceTag {
	return c.serviceTag
}

func (c *DefinedCommand) FullSchema() bool {
	return c.fullSchema
}

func (c *FetchCommand) ActionTag() names.ActionTag {
	return c.actionTag
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

func (c *DoCommand) IsAsync() bool {
	return c.async
}
