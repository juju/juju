// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	actionapi "github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

func newRunCommand() cmd.Command {
	return modelcmd.Wrap(&runCommand{})
}

// runCommand is responsible for running arbitrary commands on remote machines.
type runCommand struct {
	modelcmd.ModelCommandBase
	out      cmd.Output
	all      bool
	timeout  time.Duration
	machines []string
	services []string
	units    []string
	commands string
}

const runDoc = `
Run the commands on the specified targets.

Targets are specified using either machine ids, service names or unit
names.  At least one target specifier is needed.

Multiple values can be set for --machine, --service, and --unit by using
comma separated values.

If the target is a machine, the command is run as the "ubuntu" user on
the remote machine.

If the target is a service, the command is run on all units for that
service. For example, if there was a service "mysql" and that service
had two units, "mysql/0" and "mysql/1", then
  --service mysql
is equivalent to
  --unit mysql/0,mysql/1

Commands run for services or units are executed in a 'hook context' for
the unit.

--all is provided as a simple way to run the command on all the machines
in the model.  If you specify --all you cannot provide additional
targets.

`

func (c *runCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "run",
		Args:    "<commands>",
		Purpose: "run the commands on the remote targets specified",
		Doc:     runDoc,
	}
}

func (c *runCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.all, "all", false, "run the commands on all the machines")
	f.DurationVar(&c.timeout, "timeout", 5*time.Minute, "how long to wait before the remote command is considered to have failed")
	f.Var(cmd.NewStringsValue(nil, &c.machines), "machine", "one or more machine ids")
	f.Var(cmd.NewStringsValue(nil, &c.services), "service", "one or more service names")
	f.Var(cmd.NewStringsValue(nil, &c.units), "unit", "one or more unit ids")
}

func (c *runCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no commands specified")
	}
	c.commands, args = args[0], args[1:]

	if c.all {
		if len(c.machines) != 0 {
			return fmt.Errorf("You cannot specify --all and individual machines")
		}
		if len(c.services) != 0 {
			return fmt.Errorf("You cannot specify --all and individual services")
		}
		if len(c.units) != 0 {
			return fmt.Errorf("You cannot specify --all and individual units")
		}
	} else {
		if len(c.machines) == 0 && len(c.services) == 0 && len(c.units) == 0 {
			return fmt.Errorf("You must specify a target, either through --all, --machine, --service or --unit")
		}
	}

	var nameErrors []string
	for _, machineId := range c.machines {
		if !names.IsValidMachine(machineId) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid machine id", machineId))
		}
	}
	for _, service := range c.services {
		if !names.IsValidService(service) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid service name", service))
		}
	}
	for _, unit := range c.units {
		if !names.IsValidUnit(unit) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid unit name", unit))
		}
	}
	if len(nameErrors) > 0 {
		return fmt.Errorf("The following run targets are not valid:\n%s",
			strings.Join(nameErrors, "\n"))
	}

	return cmd.CheckEmpty(args)
}

// ConvertActionResults takes the results from the api and creates a map
// suitable for format converstion to YAML or JSON.
func ConvertActionResults(result params.ActionResult) map[string]interface{} {
	values := make(map[string]interface{})
	if result.Error != nil {
		// Convert the error string back into an error object.
		values["Error"] = result.Error.Error()
		return values
	}
	tag, err := names.ParseTag(result.Action.Receiver)
	if err != nil {
		values["Error"] = err.Error()
		return values
	}
	values["Receiver"] = tag.Id()
	if result.Message != "" {
		values["Message"] = result.Message
	}
	// We always want to have a string for stdout, but only show stderr,
	// code and error if they are there.
	if res, ok := result.Output["Stdout"].(string); ok {
		values["Stdout"] = res
	} else {
		values["Stdout"] = ""
	}
	if res, ok := result.Output["Stderr"].(string); ok && res != "" {
		values["Stderr"] = res
	}
	if res, ok := result.Output["Code"].(float64); ok && res != 0 {
		values["Code"] = res
	}
	return values
}

func (c *runCommand) Run(ctx *cmd.Context) error {
	client, err := getRunAPIClient(c)
	if err != nil {
		return err
	}
	defer client.Close()

	var runResults []params.ActionResult
	if c.all {
		runResults, err = client.RunOnAllMachines(c.commands, c.timeout)
	} else {
		params := params.RunParams{
			Commands: c.commands,
			Timeout:  c.timeout,
			Machines: c.machines,
			Services: c.services,
			Units:    c.units,
		}
		runResults, err = client.Run(params)
	}

	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	// We want to wait for the action results indefinitely.  Discard the tick.
	wait := time.NewTimer(0 * time.Second)
	_ = <-wait.C

	// If we are just dealing with one result, AND we are using the smart
	// format, then pretend we were running it locally.
	if len(runResults) == 1 && c.out.Name() == "smart" {
		result := runResults[0]

		if result.Error != nil {
			return errors.Trace(result.Error)
		}

		tag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		result, err = getActionResult(client, tag.Id(), wait)
		if err != nil {
			return errors.Trace(err)
		}
		if result.Error != nil {
			return result.Error
		}
		if result.Message != "" {
			return fmt.Errorf("%s", result.Message)
		}
		if res, ok := result.Output["Stdout"].(string); ok {
			ctx.Stdout.Write([]byte(res))
		}
		if res, ok := result.Output["Stderr"].(string); ok && res != "" {
			ctx.Stderr.Write([]byte(res))
		}
		if res, ok := result.Output["Code"].(float64); ok {
			code := int(res)
			if code != 0 {
				return cmd.NewRcPassthroughError(code)
			}
		}
		return nil
	}

	// In case the command fails we dump *all* the enqueued id's to stderr.
	// Normally, there is enough info dumped to stdout to show which action failed.
	idValues := []actionReceiverID{}
	for _, result := range runResults {
		if result.Error != nil {
			return result.Error
		}

		if result.Action == nil {
			return errors.New("action failed to enqueue")
		}

		actionTag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			return err
		}
		receiverTag, err := names.ParseTag(result.Action.Receiver)
		if err != nil {
			return err
		}
		idValues = append(idValues, actionReceiverID{
			receiverID: receiverTag.Id(),
			actionID:   actionTag.Id(),
		})
	}

	printIDsToStderr := func() {
		for _, el := range idValues {
			fmt.Fprintf(ctx.GetStderr(), "Receiver %s: action ID %s\n", el.receiverID, el.actionID)
		}
	}

	var once sync.Once
	var wg sync.WaitGroup
	values := make([]interface{}, len(runResults))
	for i, res := range runResults {
		wg.Add(1)
		go func(i int, ar params.ActionResult) {
			defer wg.Done()
			tag, err := names.ParseActionTag(ar.Action.Tag)
			if err != nil {
				once.Do(printIDsToStderr)
				values[i] = map[string]interface{}{
					"error": err.Error(),
				}
				return
			}

			actionResult, err := getActionResult(client, tag.Id(), wait)
			if err != nil {
				once.Do(printIDsToStderr)
				values[i] = map[string]interface{}{
					"actionId": tag.Id(),
					"error":    err.Error(),
				}
				return
			}

			out := ConvertActionResults(actionResult)
			values[i] = out
		}(i, res)
	}

	wg.Wait()

	c.out.Write(ctx, values)

	return nil
}

type actionReceiverID struct {
	receiverID string
	actionID   string
}

// RunClient exposes the capabilities required by the CLI
type RunClient interface {
	action.APIClient
	RunOnAllMachines(commands string, timeout time.Duration) ([]params.ActionResult, error)
	Run(params.RunParams) ([]params.ActionResult, error)
}

// In order to be able to easily mock out the API side for testing,
// the API client is retrieved using a function.
var getRunAPIClient = func(c *runCommand) (RunClient, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return actionapi.NewClient(root), errors.Trace(err)
}

// getActionResult abstracts over the action CLI function that we use here to fetch results
var getActionResult = func(c RunClient, actionId string, wait *time.Timer) (params.ActionResult, error) {
	return action.GetActionResult(c, actionId, wait)
}
