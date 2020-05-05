// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/utils"

	actionapi "github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
)

// leaderSnippet is a regular expression for unit ID-like syntax that is used
// to indicate the current leader for an application.
const leaderSnippet = "(" + names.ApplicationSnippet + ")/leader"

var validLeader = regexp.MustCompile("^" + leaderSnippet + "$")

func newDefaultRunCommand(store jujuclient.ClientStore) cmd.Command {
	return newExecCommand(store, time.After, true)
}

func newDefaultExecCommand(store jujuclient.ClientStore) cmd.Command {
	return newExecCommand(store, time.After, false)
}

func newExecCommand(store jujuclient.ClientStore, timeAfter func(time.Duration) <-chan time.Time, compat bool) cmd.Command {
	cmd := modelcmd.Wrap(&execCommand{
		timeAfter: timeAfter,
		compat:    compat,
	})
	cmd.SetClientStore(store)
	return cmd
}

// execCommand is responsible for running arbitrary commands on remote machines.
type execCommand struct {
	modelcmd.ModelCommandBase
	out          cmd.Output
	compat       bool
	all          bool
	operator     bool
	timeout      time.Duration
	machines     []string
	applications []string
	units        []string
	commands     string
	timeAfter    func(time.Duration) <-chan time.Time
}

const execDoc = `
Run a shell command on the specified targets. Only admin users of a model
are able to use this command.

Targets are specified using either machine ids, application names or unit
names.  At least one target specifier is needed.

Multiple values can be set for --machine, --application, and --unit by using
comma separated values.

If the target is a machine, the command is run as the "root" user on
the remote machine.

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

Since juju exec creates actions, you can query for the status of commands
started with juju run by calling "juju show-action-status --name juju-run".

If you need to pass options to the command being run, you must precede the
command and its arguments with "--", to tell "juju exec" to stop processing
those arguments. For example:

    juju exec --all -- hostname -f

`

func (c *execCommand) Info() *cmd.Info {
	info := jujucmd.Info(&cmd.Info{
		Name:    "exec",
		Args:    "<commands>",
		Purpose: "Run the commands on the remote targets specified.",
		Doc:     execDoc,
	})
	if c.compat {
		info.Name = "run"
		info.Doc = strings.Replace(info.Doc, "juju exec", "juju run", -1)
	}
	return info
}

func (c *execCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "default", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
		// default is used to format a single result specially.
		"default": cmd.FormatYaml,
	})
	f.BoolVar(&c.all, "all", false, "Run the commands on all the machines")
	f.BoolVar(&c.operator, "operator", false, "Run the commands on the operator (k8s-only)")
	f.DurationVar(&c.timeout, "timeout", 5*time.Minute, "How long to wait before the remote command is considered to have failed")
	f.Var(cmd.NewStringsValue(nil, &c.machines), "machine", "One or more machine ids")
	f.Var(cmd.NewStringsValue(nil, &c.applications), "a", "One or more application names")
	f.Var(cmd.NewStringsValue(nil, &c.applications), "app", "")
	f.Var(cmd.NewStringsValue(nil, &c.applications), "application", "")
	f.Var(cmd.NewStringsValue(nil, &c.units), "u", "One or more unit ids")
	f.Var(cmd.NewStringsValue(nil, &c.units), "unit", "")
}

func (c *execCommand) Init(args []string) error {
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

// ConvertActionResults takes the results from the api and creates a map
// suitable for format conversion to YAML or JSON.
func ConvertActionResults(result params.ActionResult, query actionQuery, compat bool) map[string]interface{} {
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
		messageKey := "message"
		if compat {
			messageKey = "Message"
		}
		values[messageKey] = result.Message
	}
	val := action.ConvertActionOutput(result.Output, compat, true)
	for k, v := range val {
		values[k] = v
	}
	if unit, ok := values["UnitId"]; ok && !compat {
		delete(values, "UnitId")
		values["unit"] = unit
	}
	if unit, ok := values["unit"]; ok && compat {
		delete(values, "unit")
		values["UnitId"] = unit
	}
	return values
}

func (c *execCommand) Run(ctx *cmd.Context) error {
	client, err := getExecAPIClient(c)
	if err != nil {
		return err
	}
	defer client.Close()

	modelType, err := c.ModelType()
	if err != nil {
		return errors.Annotatef(err, "unable to get model type")
	}

	if modelType == model.CAAS {
		if client.BestAPIVersion() < 4 {
			return errors.Errorf("k8s controller does not support juju exec" +
				"\nconsider upgrading your controller")
		}
		if len(c.machines) > 0 {
			return errors.Errorf("unable to target machines with a k8s controller")
		}
	}

	var runResults []params.ActionResult
	if c.all {
		runResults, err = client.RunOnAllMachines(c.commands, c.timeout)
	} else {
		// Make sure the server supports <application>/leader syntax
		for _, unit := range c.units {
			if validLeader.MatchString(unit) && client.BestAPIVersion() < 3 {
				app := strings.Split(unit, "/")[0]
				return errors.Errorf("unable to determine leader for application %q"+
					"\nleader determination is unsupported by this API"+
					"\neither upgrade your controller, or explicitly specify a unit", app)
			}
		}

		params := params.RunParams{
			Commands:     c.commands,
			Timeout:      c.timeout,
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
			params.WorkloadContext = !c.operator
		}
		runResults, err = client.Run(params)
	}

	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	actionsToQuery := []actionQuery{}
	for _, result := range runResults {
		if result.Error != nil {
			fmt.Fprintf(ctx.GetStderr(), "couldn't queue one action: %v\n", result.Error)
			continue
		}
		actionTag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			fmt.Fprintf(ctx.GetStderr(), "got invalid action tag %v for receiver %v\n", result.Action.Tag, result.Action.Receiver)
			continue
		}
		receiverTag, err := names.ActionReceiverFromTag(result.Action.Receiver)
		if err != nil {
			fmt.Fprintf(ctx.GetStderr(), "got invalid action receiver tag %v for action %v\n", result.Action.Receiver, result.Action.Tag)
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

	timeout := c.timeAfter(c.timeout)
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

			values = append(values, ConvertActionResults(result, actionsToQuery[i], c.compat))
		}
		actionsToQuery = newActionsToQuery

		if len(actionsToQuery) > 0 {
			var timedOut bool
			select {
			case <-timeout:
				timedOut = true
			case <-c.timeAfter(1 * time.Second):
				// TODO(axw) 2017-02-07 #1662451
				// use a watcher instead of polling.
				// this should be easier once we implement
				// action grouping
			}
			if timedOut {
				break
			}
		}
	}

	// If we are just dealing with one result, AND we are using the default
	// format, then pretend we were running it locally.
	if len(actionsToQuery) == 0 && len(values) == 1 && c.out.Name() == "default" {
		result, ok := values[0].(map[string]interface{})
		if !ok {
			return errors.New("couldn't read action output")
		}
		if res, ok := result["Error"].(string); ok {
			return errors.New(res)
		}
		stdoutKey := "stdout"
		if c.compat {
			stdoutKey = "Stdout"
		}
		stderrKey := "stderr"
		if c.compat {
			stderrKey = "Stderr"
		}
		codeKey := "return-code"
		if c.compat {
			codeKey = "ReturnCode"
		}
		ctx.Stdout.Write(formatOutput(result, stdoutKey, c.compat))
		ctx.Stderr.Write(formatOutput(result, stderrKey, c.compat))
		if code, ok := result[codeKey].(int); ok && code != 0 {
			return cmd.NewRcPassthroughError(code)
		}
		// Message should always contain only errors.
		messageKey := "message"
		if c.compat {
			messageKey = "Message"
		}
		if res, ok := result[messageKey].(string); ok && res != "" {
			ctx.Stderr.Write([]byte(res))
		}

		return nil
	}

	if len(values) > 0 {
		if err := c.out.Write(ctx, values); err != nil {
			return err
		}
	}

	if n := len(actionsToQuery); n > 0 {
		// There are action results remaining, so return an error.
		suffix := ""
		if n > 1 {
			suffix = "s"
		}
		receivers := make([]string, n)
		for i, actionToQuery := range actionsToQuery {
			receivers[i] = names.ReadableString(actionToQuery.receiver.tag)
		}
		return errors.Errorf(
			"timed out waiting for result%s from: %s",
			suffix, strings.Join(receivers, ", "),
		)
	}
	return nil
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
type ExecClient interface {
	action.APIClient
	RunOnAllMachines(commands string, timeout time.Duration) ([]params.ActionResult, error)
	Run(params.RunParams) ([]params.ActionResult, error)
}

// In order to be able to easily mock out the API side for testing,
// the API client is retrieved using a function.
var getExecAPIClient = func(c *execCommand) (ExecClient, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return actionapi.NewClient(root), errors.Trace(err)
}

// getActionResult abstracts over the action CLI function that we use here to fetch results
var getActionResult = func(c ExecClient, actionId string, wait *time.Timer) (params.ActionResult, error) {
	return action.GetActionResult(c, actionId, wait, false)
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

func formatOutput(results map[string]interface{}, key string, compat bool) []byte {
	res, ok := results[key].(string)
	if !ok {
		return []byte("")
	}
	encodingKey := "-encoding"
	if compat {
		encodingKey = ".encoding"
	}
	if enc, ok := results[key+encodingKey].(string); ok && enc != "" {
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
