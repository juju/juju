// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var (
	NewActionAPIClient = &newAPIClient
	AddValueToMap      = addValueToMap
)

type ShowOutputCommand struct {
	*showOutputCommand
}

type StatusCommand struct {
	*statusCommand
}

type RunCommand struct {
	*runCommand
}

func (c *RunCommand) UnitTag() names.UnitTag {
	return c.unitTag
}

func (c *RunCommand) ActionName() string {
	return c.actionName
}

func (c *RunCommand) ParseStrings() bool {
	return c.parseStrings
}

func (c *RunCommand) ParamsYAML() cmd.FileVar {
	return c.paramsYAML
}

func (c *RunCommand) Args() [][]string {
	return c.args
}

type ListCommand struct {
	*listCommand
}

func (c *ListCommand) ServiceTag() names.ServiceTag {
	return c.serviceTag
}

func (c *ListCommand) FullSchema() bool {
	return c.fullSchema
}

func NewShowOutputCommandForTest(store jujuclient.ClientStore) (cmd.Command, *ShowOutputCommand) {
	c := &showOutputCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &ShowOutputCommand{c}
}

func NewStatusCommandForTest(store jujuclient.ClientStore) (cmd.Command, *StatusCommand) {
	c := &statusCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &StatusCommand{c}
}

func NewListCommandForTest(store jujuclient.ClientStore) (cmd.Command, *ListCommand) {
	c := &listCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.ModelSkipDefault), &ListCommand{c}
}

func NewRunCommandForTest(store jujuclient.ClientStore) (cmd.Command, *RunCommand) {
	c := &runCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.ModelSkipDefault), &RunCommand{c}
}

func ActionResultsToMap(results []params.ActionResult) map[string]interface{} {
	return resultsToMap(results)
}
