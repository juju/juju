// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd/v3"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const showMachineCommandDoc = `
Show a specified machine on a model.  The default format is ` + "`yaml`" + `;
other formats can be specified with the ` + "`--format`" + ` option.
Available formats are ` + "`yaml`" + `, ` + "`tabular`" + `, and ` + "`json`" + `.
`

const showMachineExamples = `
    juju show-machine 0
    juju show-machine 1 2 3
`

// NewShowMachineCommand returns a command that shows details on the specified machine[s].
func NewShowMachineCommand() cmd.Command {
	return modelcmd.Wrap(newShowMachineCommand(nil))
}

func newShowMachineCommand(api statusAPI) *showMachineCommand {
	showCmd := &showMachineCommand{}
	showCmd.defaultFormat = "yaml"
	showCmd.api = api
	return showCmd
}

// showMachineCommand struct holds details on the specified machine[s].
type showMachineCommand struct {
	baselistMachinesCommand
}

// Info implements Command.Info.
func (c *showMachineCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-machine",
		Args:     "<machineID> ...",
		Purpose:  "Show a machine's status.",
		Doc:      showMachineCommandDoc,
		Examples: showMachineExamples,
		SeeAlso: []string{
			"add-machine",
		},
	})
}

// Init captures machineId's to show from CL args.
func (c *showMachineCommand) Init(args []string) error {
	c.machineIds = args
	return nil
}
