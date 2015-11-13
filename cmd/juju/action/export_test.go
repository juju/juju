// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

var (
	NewActionAPIClient = &newAPIClient
	AddValueToMap      = addValueToMap
	NewFetchCommand    = newFetchCommand
	NewStatusCommand   = newStatusCommand
)

type DoCommand struct {
	*doCommand
}

func (c *DoCommand) UnitTag() names.UnitTag {
	return c.unitTag
}

func (c *DoCommand) ActionName() string {
	return c.actionName
}

func (c *DoCommand) ParseStrings() bool {
	return c.parseStrings
}

func (c *DoCommand) ParamsYAML() cmd.FileVar {
	return c.paramsYAML
}

func (c *DoCommand) Args() [][]string {
	return c.args
}

type DefinedCommand struct {
	*definedCommand
}

func (c *DefinedCommand) ServiceTag() names.ServiceTag {
	return c.serviceTag
}

func (c *DefinedCommand) FullSchema() bool {
	return c.fullSchema
}

func NewDefinedCommand() (cmd.Command, *DefinedCommand) {
	c := &definedCommand{}
	return envcmd.Wrap(c, envcmd.EnvSkipDefault), &DefinedCommand{c}
}

func NewDoCommand() (cmd.Command, *DoCommand) {
	c := &doCommand{}
	return envcmd.Wrap(c, envcmd.EnvSkipDefault), &DoCommand{c}
}
func ActionResultsToMap(results []params.ActionResult) map[string]interface{} {
	return resultsToMap(results)
}
