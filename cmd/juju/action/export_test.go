// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v4"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var (
	NewActionAPIClient = &newAPIClient
	AddValueToMap      = addValueToMap
)

type ShowOperationCommand struct {
	*showOperationCommand
}

type CancelCommand struct {
	*cancelCommand
}

type ExecCommand struct {
	*execCommand
}

func (c *ExecCommand) Wait() time.Duration {
	return c.wait
}

func (c *ExecCommand) All() bool {
	return c.all
}

func (c *ExecCommand) Machines() []string {
	return c.machines
}

func (c *ExecCommand) Applications() []string {
	return c.applications
}

func (c *ExecCommand) Units() []string {
	return c.units
}

func (c *ExecCommand) Commands() string {
	return c.commands
}

type RunCommand struct {
	*runCommand
}

func (c *RunCommand) UnitNames() []string {
	return c.unitReceivers
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

func (c *RunCommand) Wait() time.Duration {
	return c.wait
}

func (c *RunCommand) Args() [][]string {
	return c.args
}

type ListCommand struct {
	*listCommand
}

func (c *ListCommand) ApplicationName() string {
	return c.appName
}

func (c *ListCommand) FullSchema() bool {
	return c.fullSchema
}

type ShowCommand struct {
	*showCommand
}

func (c *ShowCommand) ApplicationName() string {
	return c.appName
}

func (c *ShowCommand) ActionName() string {
	return c.actionName
}

type ListOperationsCommand struct {
	*listOperationsCommand
}

func NewShowTaskCommandForTest(store jujuclient.ClientStore, clock clock.Clock, logMessage func(*cmd.Context, string)) (cmd.Command, *showTaskCommand) {
	c := &showTaskCommand{
		logMessageHandler: logMessage,
		clock:             clock,
	}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), c
}

func NewShowOperationCommandForTest(store jujuclient.ClientStore, clock clock.Clock) (cmd.Command, *ShowOperationCommand) {
	c := &showOperationCommand{
		clock: clock,
	}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &ShowOperationCommand{c}
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

func NewRunCommandForTest(store jujuclient.ClientStore, clock clock.Clock, logMessage func(*cmd.Context, string)) (cmd.Command, *RunCommand) {
	c := &runCommand{
		runCommandBase: runCommandBase{
			logMessageHandler: logMessage,
			clock:             clock,
		},
	}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &RunCommand{c}
}

func NewExecCommandForTest(store jujuclient.ClientStore, clock clock.Clock, logMessageHandler func(*cmd.Context, string)) (cmd.Command, *ExecCommand) {
	c := &execCommand{
		runCommandBase: runCommandBase{
			defaultWait:       5 * time.Minute,
			logMessageHandler: logMessageHandler,
			clock:             clock,
			hideProgress:      true,
		},
	}
	c.SetClientStore(store)
	return modelcmd.Wrap(c), &ExecCommand{c}
}

func ActionResultsToMap(results []actionapi.ActionResult) map[string]interface{} {
	return resultsToMap(results)
}

func NewListOperationsCommandForTest(store jujuclient.ClientStore) (cmd.Command, *ListOperationsCommand) {
	c := &listOperationsCommand{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c, modelcmd.WrapSkipDefaultModel), &ListOperationsCommand{c}
}
