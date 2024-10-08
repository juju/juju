// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"strings"

	"github.com/juju/clock"
	"github.com/juju/cmd/v4"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"gopkg.in/yaml.v2"

	actionapi "github.com/juju/juju/api/client/action"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewRunCommand() cmd.Command {
	return modelcmd.Wrap(&runCommand{
		runCommandBase: runCommandBase{
			logMessageHandler: func(ctx *cmd.Context, msg string) {
				ctx.Infof("%s", msg)
			},
			clock: clock.WallClock,
		},
	})
}

// runCommand enqueues an Action for running on the given unit with given
// params
type runCommand struct {
	runCommandBase
	unitReceivers []string
	actionName    string
	paramsYAML    cmd.FileVar
	parseStrings  bool
	args          [][]string
}

const runDoc = `
Run a charm action for execution on the given unit(s), with a given set of params.
An ID is returned for use with 'juju show-operation <ID>'.

All units must be of the same application.

A action executed on a given unit becomes a task with an ID that can be
used with 'juju show-task <ID>'.

Running an action returns the overall operation ID as well as the individual
task ID(s) for each unit.

To queue a action to be run in the background without waiting for it to finish,
use the --background option.

To set the maximum time to wait for a action to complete, use the --wait option.

By default, a single action will output its failure message if the action fails,
followed by any results set by the action. For multiple actions, each action's
results will be printed with the action id and action status. To see more detailed
information about run timings etc, use --format yaml.

Valid unit identifiers are: 
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If the leader syntax is used, the leader unit for the application will be
resolved before the action is enqueued.

Params are validated according to the charm for the unit's application.  The
valid params can be seen using "juju actions <application> --schema".
Params may be in a yaml file which is passed with the --params option, or they
may be specified by a key.key.key...=value format (see examples below.)

Params given in the CLI invocation will be parsed as YAML unless the
--string-args option is set.  This can be helpful for values such as 'y', which
is a boolean true in YAML.

If --params is passed, along with key.key...=value explicit arguments, the
explicit arguments will override the parameter file.
`

const runExamples = `
    juju run mysql/3 backup --background
    juju run mysql/3 backup --wait=2m
    juju run mysql/3 backup --format yaml
    juju run mysql/3 backup --utc
    juju run mysql/3 backup
    juju run mysql/leader backup
    juju show-operation <ID>
    juju run mysql/3 backup --params parameters.yml
    juju run mysql/3 backup out=out.tar.bz2 file.kind=xz file.quality=high
    juju run mysql/3 backup --params p.yml file.kind=xz file.quality=high
    juju run sleeper/0 pause time=1000
    juju run sleeper/0 pause --string-args time=1000
`

// SetFlags offers an option for YAML output.
func (c *runCommand) SetFlags(f *gnuflag.FlagSet) {
	c.runCommandBase.SetFlags(f)

	f.Var(&c.paramsYAML, "params", "Path to yaml-formatted params file")
	f.BoolVar(&c.parseStrings, "string-args", false, "Use raw string values of CLI args")
}

func (c *runCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "run",
		Args:     "<unit> [<unit> ...] <action-name> [<key>=<value> [<key>[.<key> ...]=<value>]]",
		Purpose:  "Run an action on a specified unit.",
		Doc:      runDoc,
		Examples: runExamples,
		SeeAlso: []string{
			"operations",
			"show-operation",
			"show-task",
		},
	})
}

// Init gets the unit tag(s), action name and action arguments.
func (c *runCommand) Init(args []string) (err error) {
	if err := c.runCommandBase.Init(args); err != nil {
		return errors.Trace(err)
	}
	applicationNames := set.NewStrings()
	for _, arg := range args {
		if s := validUnitOrLeader.FindStringSubmatch(arg); s != nil {
			applicationNames.Add(s[1])
			c.unitReceivers = append(c.unitReceivers, arg)
		} else if nameRule.MatchString(arg) {
			c.actionName = arg
			break
		} else {
			return errors.Errorf("invalid unit or action name %q", arg)
		}
	}
	if len(c.unitReceivers) == 0 {
		return errors.New("no unit specified")
	}
	if c.actionName == "" {
		return errors.New("no action specified")
	}
	if len(applicationNames) > 1 {
		return errors.New("all units must be of the same application")
	}

	// Parse CLI key-value args if they exist.
	c.args = make([][]string, 0)
	for _, arg := range args[len(c.unitReceivers)+1:] {
		thisArg := strings.SplitN(arg, "=", 2)
		if len(thisArg) != 2 {
			return errors.Errorf("argument %q must be of the form key.key.key...=value", arg)
		}
		keySlice := strings.Split(thisArg[0], ".")
		// check each key for validity
		for _, key := range keySlice {
			if valid := nameRule.MatchString(key); !valid {
				return errors.Errorf("key %q must start and end with lowercase alphanumeric, "+
					"and contain only lowercase alphanumeric and hyphens", key)
			}
		}
		c.args = append(c.args, append(keySlice, thisArg[1]))
	}
	return nil
}

func (c *runCommand) Run(ctx *cmd.Context) error {
	if err := c.ensureAPI(ctx); err != nil {
		return errors.Trace(err)
	}
	defer c.api.Close()

	results, err := c.enqueueActions(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return c.operationResults(ctx, results)
}

func (c *runCommand) enqueueActions(ctx *cmd.Context) (*actionapi.EnqueuedActions, error) {
	actionParams := map[string]interface{}{}
	if c.paramsYAML.Path != "" {
		b, err := c.paramsYAML.Read(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}

		err = yaml.Unmarshal(b, &actionParams)
		if err != nil {
			return nil, errors.Trace(err)
		}

		conformantParams, err := common.ConformYAML(actionParams)
		if err != nil {
			return nil, errors.Trace(err)
		}

		betterParams, ok := conformantParams.(map[string]interface{})
		if !ok {
			return nil, errors.New("params must contain a YAML map with string keys")
		}

		actionParams = betterParams
	}
	// If we had explicit args {..., [key, key, key, key, value], ...}
	// then iterate and set params ..., key.key.key.key=value, ...
	for _, argSlice := range c.args {
		valueIndex := len(argSlice) - 1
		keys := argSlice[:valueIndex]
		value := argSlice[valueIndex]
		cleansedValue := interface{}(value)
		if !c.parseStrings {
			err := yaml.Unmarshal([]byte(value), &cleansedValue)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		// Insert the value in the map.
		addValueToMap(keys, cleansedValue, actionParams)
	}
	conformantParams, err := common.ConformYAML(actionParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	typedConformantParams, ok := conformantParams.(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("params must be a map, got %T", typedConformantParams)
	}
	actions := make([]actionapi.Action, len(c.unitReceivers))
	for i, unitReceiver := range c.unitReceivers {
		if strings.HasSuffix(unitReceiver, "leader") {
			actions[i].Receiver = unitReceiver
		} else {
			actions[i].Receiver = names.NewUnitTag(unitReceiver).String()
		}
		actions[i].Name = c.actionName
		actions[i].Parameters = actionParams
	}
	results, err := c.api.EnqueueOperation(ctx, actions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Actions) != len(c.unitReceivers) {
		return nil, errors.New("illegal number of results returned")
	}
	return &results, nil
}
