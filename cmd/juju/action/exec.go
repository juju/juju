// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v2"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
)

// NewExecCommand returns an exec command.
func NewExecCommand(store jujuclient.ClientStore) cmd.Command {
	logMessageHandler := func(ctx *cmd.Context, msg string) {
		ctx.Infof(msg)
	}
	return newExecCommand(store, logMessageHandler, clock.WallClock)
}

func newExecCommand(store jujuclient.ClientStore, logMessageHandler func(*cmd.Context, string), clock clock.Clock) cmd.Command {
	cmd := modelcmd.Wrap(&execCommand{
		runCommandBase: runCommandBase{
			defaultWait:       5 * time.Minute,
			logMessageHandler: logMessageHandler,
			clock:             clock,
		},
	})
	cmd.SetClientStore(store)
	return cmd
}

// execCommand is responsible for running arbitrary commands on remote machines.
type execCommand struct {
	runCommandBase
	all          bool
	operator     bool
	machines     []string
	applications []string
	units        []string
	commands     string
}

const execDoc = `
Run a shell command on the specified targets. Only admin users of a model
are able to use this command.

Targets are specified using either machine ids, application names or unit
names.  At least one target specifier is needed.

Multiple values can be set for --machine, --application, and --unit by using
comma separated values.

Depending on the type of target, the user which the command runs as will be:
  unit -> "root"
  machine -> "ubuntu"
The target and user are independent of whether --all or --application are used.
For example, --all will run as "ubuntu" on machines and "root" on units.
And --application will run as "root" on all units of that application.

Some options are shortened for usabilty purpose in CLI
--application can also be specified as --app and -a
--unit can also be specified as -u

Valid unit identifiers are: 
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If the target is an application, the command is run on all units for that
application. For example, if there was an application "mysql" and that application
had two units, "mysql/0" and "mysql/1", then
  --application mysql
is equivalent to
  --unit mysql/0,mysql/1

If --operator is provided on k8s models, commands are executed on the operator
instead of the workload. On IAAS models, --operator has no effect.

Commands run for applications or units are executed in a 'hook context' for
the unit.

--all is provided as a simple way to run the command on all the machines
in the model.  If you specify --all you cannot provide additional
targets.

Since juju exec creates tasks, you can query for the status of commands
started with juju run by calling "juju operations --machines <id>,... --actions juju-run".

If you need to pass options to the command being run, you must precede the
command and its arguments with "--", to tell "juju exec" to stop processing
those arguments. For example:

    juju exec --all -- hostname -f

`

// Info implements Command.Info.
func (c *execCommand) Info() *cmd.Info {
	info := jujucmd.Info(&cmd.Info{
		Name:    "exec",
		Args:    "<commands>",
		Purpose: "Run the commands on the remote targets specified.",
		Doc:     execDoc,
	})
	return info
}

// SetFlags implements Command.SetFlags.
func (c *execCommand) SetFlags(f *gnuflag.FlagSet) {
	c.runCommandBase.SetFlags(f)
	f.BoolVar(&c.all, "all", false, "Run the commands on all the machines")
	f.BoolVar(&c.operator, "operator", false, "Run the commands on the operator (k8s-only)")
	f.Var(cmd.NewStringsValue(nil, &c.machines), "machine", "One or more machine ids")
	f.Var(cmd.NewStringsValue(nil, &c.applications), "a", "One or more application names")
	f.Var(cmd.NewStringsValue(nil, &c.applications), "app", "")
	f.Var(cmd.NewStringsValue(nil, &c.applications), "application", "")
	f.Var(cmd.NewStringsValue(nil, &c.units), "u", "One or more unit ids")
	f.Var(cmd.NewStringsValue(nil, &c.units), "unit", "")
}

// Init implements Command.Init.
func (c *execCommand) Init(args []string) error {
	if err := c.runCommandBase.Init(args); err != nil {
		return errors.Trace(err)
	}
	if len(args) == 0 {
		return errors.Errorf("no commands specified")
	}
	if len(args) == 1 {
		// If just one argument is specified, we don't pass it through
		// utils.CommandString in case it contains multiple arguments
		// (e.g. juju run --all "sudo whatever"). Passing it through
		// utils.CommandString would quote the string, which the backend
		// does not expect.
		c.commands = args[0]
	} else {
		c.commands = utils.CommandString(args...)
	}

	if c.all {
		if len(c.machines) != 0 {
			return errors.Errorf("You cannot specify --all and individual machines")
		}
		if len(c.applications) != 0 {
			return errors.Errorf("You cannot specify --all and individual applications")
		}
		if len(c.units) != 0 {
			return errors.Errorf("You cannot specify --all and individual units")
		}
	} else {
		if len(c.machines) == 0 && len(c.applications) == 0 && len(c.units) == 0 {
			return errors.Errorf("You must specify a target, either through --all, --machine, --application or --unit")
		}
	}

	var nameErrors []string
	for _, machineId := range c.machines {
		if !names.IsValidMachine(machineId) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid machine id", machineId))
		}
	}
	for _, application := range c.applications {
		if !names.IsValidApplication(application) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid application name", application))
		}
	}
	for _, unit := range c.units {
		if validLeader.MatchString(unit) {
			continue
		}

		if !names.IsValidUnit(unit) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid unit name", unit))
		}
	}
	if len(nameErrors) > 0 {
		return errors.Errorf("The following exec targets are not valid:\n%s",
			strings.Join(nameErrors, "\n"))
	}

	return nil
}

// Run implements Command.Run.
func (c *execCommand) Run(ctx *cmd.Context) error {
	if err := c.ensureAPI(); err != nil {
		return errors.Trace(err)
	}
	defer c.api.Close()

	modelType, err := c.ModelType()
	if err != nil {
		return errors.Annotatef(err, "unable to get model type")
	}

	if modelType == model.CAAS {
		if len(c.machines) > 0 {
			return errors.Errorf("unable to target machines with a k8s controller")
		}
	}

	var runResults params.EnqueuedActions
	if c.all {
		runResults, err = c.api.RunOnAllMachines(c.commands, c.wait)
	} else {
		runParams := params.RunParams{
			Commands:     c.commands,
			Timeout:      c.wait,
			Machines:     c.machines,
			Applications: c.applications,
			Units:        c.units,
		}
		if c.operator {
			if modelType != model.CAAS {
				return errors.Errorf("only k8s models support the --operator flag")
			}
		}
		if modelType == model.CAAS {
			runParams.WorkloadContext = !c.operator
		}
		runResults, err = c.api.Run(runParams)
	}

	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	return c.processOperationResults(ctx, &runResults)
}
