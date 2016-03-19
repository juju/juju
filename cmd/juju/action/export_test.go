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

type FetchCommand struct {
	*fetchCommand
}

type StatusCommand struct {
	*statusCommand
}

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

func NewFetchCommand(store jujuclient.ClientStore) (cmd.Command, *FetchCommand) {
	c := &fetchCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &FetchCommand{c}
}

func NewStatusCommand(store jujuclient.ClientStore) (cmd.Command, *StatusCommand) {
	c := &statusCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &StatusCommand{c}
}

func NewDefinedCommand(store jujuclient.ClientStore) (cmd.Command, *DefinedCommand) {
	c := &definedCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.ModelSkipDefault), &DefinedCommand{c}
}

func NewDoCommand(store jujuclient.ClientStore) (cmd.Command, *DoCommand) {
	c := &doCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.ModelSkipDefault), &DoCommand{c}
}

func ActionResultsToMap(results []params.ActionResult) map[string]interface{} {
	return resultsToMap(results)
}
