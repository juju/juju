// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/cmd"
	"gopkg.in/juju/names.v3"

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

type CancelCommand struct {
	*cancelCommand
}

type CallCommand struct {
	*callCommand
}

func (c *CallCommand) UnitNames() []string {
	return c.unitReceivers
}

func (c *CallCommand) FunctionName() string {
	return c.functionName
}

func (c *CallCommand) ParseStrings() bool {
	return c.parseStrings
}

func (c *CallCommand) ParamsYAML() cmd.FileVar {
	return c.paramsYAML
}

func (c *CallCommand) MaxWait() time.Duration {
	return c.maxWait
}

func (c *CallCommand) Args() [][]string {
	return c.args
}

type RunActionCommand struct {
	*runActionCommand
}

func (c *RunActionCommand) UnitNames() []string {
	return c.unitReceivers
}

func (c *RunActionCommand) ActionName() string {
	return c.actionName
}

func (c *RunActionCommand) ParseStrings() bool {
	return c.parseStrings
}

func (c *RunActionCommand) ParamsYAML() cmd.FileVar {
	return c.paramsYAML
}

func (c *RunActionCommand) Args() [][]string {
	return c.args
}

type ListCommand struct {
	*listCommand
}

func (c *ListCommand) ApplicationTag() names.ApplicationTag {
	return c.applicationTag
}

func (c *ListCommand) FullSchema() bool {
	return c.fullSchema
}

type ShowCommand struct {
	*showCommand
}

func (c *ShowCommand) ApplicationTag() names.ApplicationTag {
	return c.applicationTag
}

func (c *ShowCommand) ActionName() string {
	return c.functionName
}

type ListTasksCommand struct {
	*listTasksCommand
}

func NewShowOutputCommandForTest(store jujuclient.ClientStore, logMessage func(*cmd.Context, string)) (cmd.Command, *ShowOutputCommand) {
	c := &showOutputCommand{
		compat:            true,
		logMessageHandler: logMessage,
	}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &ShowOutputCommand{c}
}

func NewStatusCommandForTest(store jujuclient.ClientStore) (cmd.Command, *StatusCommand) {
	c := &statusCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &StatusCommand{c}
}

func NewCancelCommandForTest(store jujuclient.ClientStore) (cmd.Command, *CancelCommand) {
	c := &cancelCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &CancelCommand{c}
}

func NewListCommandForTest(store jujuclient.ClientStore) (cmd.Command, *ListCommand) {
	c := &listCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &ListCommand{c}
}

func NewShowCommandForTest(store jujuclient.ClientStore) (cmd.Command, *ShowCommand) {
	c := &showCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &ShowCommand{c}
}

func NewCallCommandForTest(store jujuclient.ClientStore, logMessage func(*cmd.Context, string)) (cmd.Command, *CallCommand) {
	c := &callCommand{
		logMessageHandler: logMessage,
	}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &CallCommand{c}
}

func NewRunActionCommandForTest(store jujuclient.ClientStore) (cmd.Command, *RunActionCommand) {
	c := &runActionCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &RunActionCommand{c}
}

func ActionResultsToMap(results []params.ActionResult) map[string]interface{} {
	return resultsToMap(results)
}

func NewListTasksCommandForTest(store jujuclient.ClientStore) (cmd.Command, *ListTasksCommand) {
	c := &listTasksCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &ListTasksCommand{c}
}
