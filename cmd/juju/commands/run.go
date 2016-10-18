// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

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
Run the commands on the specified targets. Only admin users of a model
are able to use this command.

Targets are specified using either machine ids, application names or unit
names.  At least one target specifier is needed.

Multiple values can be set for --machine, --application, and --unit by using
comma separated values.

If the target is a machine, the command is run as the "ubuntu" user on
the remote machine.

If the target is an application, the command is run on all units for that
application. For example, if there was an application "mysql" and that application
had two units, "mysql/0" and "mysql/1", then
  --application mysql
is equivalent to
  --unit mysql/0,mysql/1

Commands run for applications or units are executed in a 'hook context' for
the unit.

--all is provided as a simple way to run the command on all the machines
in the model.  If you specify --all you cannot provide additional
targets.

Since juju run creates actions, you can query for the status of commands
started with juju run by calling "juju show-action-status --name juju-run".
`

func (c *runCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "run",
		Args:    "<commands>",
		Purpose: "Run the commands on the remote targets specified.",
		Doc:     runDoc,
	}
}

func (c *runCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "default", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
		// default is used to format a single result specially.
		"default": cmd.FormatYaml,
	})
	f.BoolVar(&c.all, "all", false, "Run the commands on all the machines")
	f.DurationVar(&c.timeout, "timeout", 5*time.Minute, "How long to wait before the remote command is considered to have failed")
	f.Var(cmd.NewStringsValue(nil, &c.machines), "machine", "One or more machine ids")
	f.Var(cmd.NewStringsValue(nil, &c.services), "application", "One or more application names")
	f.Var(cmd.NewStringsValue(nil, &c.units), "unit", "One or more unit ids")
}

func (c *runCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no commands specified")
	}
	c.commands, args = args[0], args[1:]

	if c.all {
		if len(c.machines) != 0 {
			return errors.Errorf("You cannot specify --all and individual machines")
		}
		if len(c.services) != 0 {
			return errors.Errorf("You cannot specify --all and individual applications")
		}
		if len(c.units) != 0 {
			return errors.Errorf("You cannot specify --all and individual units")
		}
	} else {
		if len(c.machines) == 0 && len(c.services) == 0 && len(c.units) == 0 {
			return errors.Errorf("You must specify a target, either through --all, --machine, --application or --unit")
		}
	}

	var nameErrors []string
	for _, machineId := range c.machines {
		if !names.IsValidMachine(machineId) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid machine id", machineId))
		}
	}
	for _, service := range c.services {
		if !names.IsValidApplication(service) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid application name", service))
		}
	}
	for _, unit := range c.units {
		if !names.IsValidUnit(unit) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid unit name", unit))
		}
	}
	if len(nameErrors) > 0 {
		return errors.Errorf("The following run targets are not valid:\n%s",
			strings.Join(nameErrors, "\n"))
	}

	return cmd.CheckEmpty(args)
}

// ConvertActionResults takes the results from the api and creates a map
// suitable for format converstion to YAML or JSON.
func ConvertActionResults(result params.ActionResult, query actionQuery) map[string]interface{} {
	values := make(map[string]interface{})
	values[query.receiver.receiverType] = query.receiver.tag.Id()
	if result.Error != nil {
		values["Error"] = result.Error.Error()
		values["Action"] = query.actionTag.Id()
		return values
	}
	if result.Action.Tag != query.actionTag.String() {
		values["Error"] = fmt.Sprintf("expected action tag %q, got %q", query.actionTag.String(), result.Action.Tag)
		values["Action"] = query.actionTag.Id()
		return values
	}
	if result.Action.Receiver != query.receiver.tag.String() {
		values["Error"] = fmt.Sprintf("expected action receiver %q, got %q", query.receiver.tag.String(), result.Action.Receiver)
		values["Action"] = query.actionTag.Id()
		return values
	}
	if result.Message != "" {
		values["Message"] = result.Message
	}
	// We always want to have a string for stdout, but only show stderr,
	// code and error if they are there.
	if res, ok := result.Output["Stdout"].(string); ok {
		values["Stdout"] = strings.Replace(res, "\r\n", "\n", -1)
		if res, ok := result.Output["StdoutEncoding"].(string); ok && res != "" {
			values["Stdout.encoding"] = res
		}
	} else {
		values["Stdout"] = ""
	}
	if res, ok := result.Output["Stderr"].(string); ok && res != "" {
		values["Stderr"] = strings.Replace(res, "\r\n", "\n", -1)
		if res, ok := result.Output["StderrEncoding"].(string); ok && res != "" {
			values["Stderr.encoding"] = res
		}
	}
	if res, ok := result.Output["Code"].(string); ok {
		code, err := strconv.Atoi(res)
		if err == nil && code != 0 {
			values["ReturnCode"] = code
		}
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
			Commands:     c.commands,
			Timeout:      c.timeout,
			Machines:     c.machines,
			Applications: c.services,
			Units:        c.units,
		}
		runResults, err = client.Run(params)
	}

	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	actionsToQuery := []actionQuery{}
	for _, result := range runResults {
		if result.Error != nil {
			fmt.Fprintf(ctx.GetStderr(), "couldn't queue one action: %v", result.Error)
			continue
		}
		actionTag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			fmt.Fprintf(ctx.GetStderr(), "got invalid action tag %v for receiver %v", result.Action.Tag, result.Action.Receiver)
			continue
		}

		receiverTag, err := names.ActionReceiverFromTag(result.Action.Receiver)
		if err != nil {
			fmt.Fprintf(ctx.GetStderr(), "got invalid action receiver tag %v for action %v", result.Action.Receiver, result.Action.Tag)
			continue
		}
		var receiverType string
		switch receiverTag.(type) {
		case names.UnitTag:
			receiverType = "UnitId"
		case names.MachineTag:
			receiverType = "MachineId"
		default:
			receiverType = "ReceiverId"
		}
		actionsToQuery = append(actionsToQuery, actionQuery{
			actionTag: actionTag,
			receiver: actionReceiver{
				receiverType: receiverType,
				tag:          receiverTag,
			}})
	}

	if len(actionsToQuery) == 0 {
		return errors.New("no actions were successfully enqueued, aborting")
	}

	values := []interface{}{}
	for len(actionsToQuery) > 0 {
		actionResults, err := client.Actions(entities(actionsToQuery))
		if err != nil {
			return errors.Trace(err)
		}

		newActionsToQuery := []actionQuery{}
		for i, result := range actionResults.Results {
			if result.Error == nil {
				switch result.Status {
				case params.ActionRunning, params.ActionPending:
					newActionsToQuery = append(newActionsToQuery, actionsToQuery[i])
					continue
				}
			}

			values = append(values, ConvertActionResults(result, actionsToQuery[i]))
		}

		actionsToQuery = newActionsToQuery

		// TODO: use a watcher instead of sleeping
		// this should be easier once we implement action grouping
		<-afterFunc(1 * time.Second)
	}

	// If we are just dealing with one result, AND we are using the default
	// format, then pretend we were running it locally.
	if len(values) == 1 && c.out.Name() == "default" {
		result, ok := values[0].(map[string]interface{})
		if !ok {
			return errors.New("couldn't read action output")
		}
		if res, ok := result["Error"].(string); ok {
			return errors.New(res)
		}
		ctx.Stdout.Write(formatOutput(result, "Stdout"))
		ctx.Stderr.Write(formatOutput(result, "Stderr"))
		if code, ok := result["ReturnCode"].(int); ok && code != 0 {
			return cmd.NewRcPassthroughError(code)
		}
		// Message should always contain only errors.
		if res, ok := result["Message"].(string); ok && res != "" {
			ctx.Stderr.Write([]byte(res))
		}

		return nil
	}

	return c.out.Write(ctx, values)
}

type actionReceiver struct {
	receiverType string
	tag          names.Tag
}

type actionQuery struct {
	receiver  actionReceiver
	actionTag names.ActionTag
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

var afterFunc = func(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// entities is a convenience constructor for params.Entities.
func entities(actions []actionQuery) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(actions)),
	}
	for i, action := range actions {
		entities.Entities[i].Tag = action.actionTag.String()
	}
	return entities
}

func formatOutput(results map[string]interface{}, key string) []byte {
	res, ok := results[key].(string)
	if !ok {
		return []byte("")
	}
	if enc, ok := results[key+".encoding"].(string); ok && enc != "" {
		switch enc {
		case "base64":
			decoded, err := base64.StdEncoding.DecodeString(res)
			if err != nil {
				return []byte("expected b64 encoded string, got " + res)
			}
			return decoded
		default:
			return []byte(res)
		}
	}
	return []byte(res)
}
