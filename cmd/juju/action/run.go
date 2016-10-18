// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"
	yaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

var keyRule = regexp.MustCompile("^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$")

func NewRunCommand() cmd.Command {
	return modelcmd.Wrap(&runCommand{})
}

// runCommand enqueues an Action for running on the given unit with given
// params
type runCommand struct {
	ActionCommandBase
	unitTag      names.UnitTag
	actionName   string
	paramsYAML   cmd.FileVar
	parseStrings bool
	out          cmd.Output
	args         [][]string
}

const runDoc = `
Queue an Action for execution on a given unit, with a given set of params.
The Action ID is returned for use with 'juju show-action-output <ID>' or
'juju show-action-status <ID>'.
 
Params are validated according to the charm for the unit's application.  The 
valid params can be seen using "juju actions <application> --schema".
Params may be in a yaml file which is passed with the --params flag, or they
may be specified by a key.key.key...=value format (see examples below.)

Params given in the CLI invocation will be parsed as YAML unless the
--string-args flag is set.  This can be helpful for values such as 'y', which
is a boolean true in YAML.

If --params is passed, along with key.key...=value explicit arguments, the
explicit arguments will override the parameter file.

Examples:

$ juju run-action mysql/3 backup 
action: <ID>

$ juju show-action-output <ID>
result:
  status: success
  file:
    size: 873.2
    units: GB
    name: foo.sql

$ juju run-action mysql/3 backup --params parameters.yml
...
Params sent will be the contents of parameters.yml.
...

$ juju run-action mysql/3 backup out=out.tar.bz2 file.kind=xz file.quality=high
...
Params sent will be:

out: out.tar.bz2
file:
  kind: xz
  quality: high
...

$ juju run-action mysql/3 backup --params p.yml file.kind=xz file.quality=high
...
If p.yml contains:

file:
  location: /var/backups/mysql/
  kind: gzip

then the merged args passed will be:

file:
  location: /var/backups/mysql/
  kind: xz
  quality: high
...

$ juju run-action sleeper/0 pause time=1000
...

$ juju run-action sleeper/0 pause --string-args time=1000
...
The value for the "time" param will be the string literal "1000".
`

// ActionNameRule describes the format an action name must match to be valid.
var ActionNameRule = regexp.MustCompile("^[a-z](?:[a-z-]*[a-z])?$")

// SetFlags offers an option for YAML output.
func (c *runCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	f.Var(&c.paramsYAML, "params", "Path to yaml-formatted params file")
	f.BoolVar(&c.parseStrings, "string-args", false, "Use raw string values of CLI args")
}

func (c *runCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "run-action",
		Args:    "<unit> <action name> [key.key.key...=value]",
		Purpose: "Queue an action for execution.",
		Doc:     runDoc,
	}
}

// Init gets the unit tag, and checks for other correct args.
func (c *runCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no unit specified")
	case 1:
		return errors.New("no action specified")
	default:
		// Grab and verify the unit and action names.
		unitName := args[0]
		if !names.IsValidUnit(unitName) {
			return errors.Errorf("invalid unit name %q", unitName)
		}
		ActionName := args[1]
		if valid := ActionNameRule.MatchString(ActionName); !valid {
			return errors.Errorf("invalid action name %q", ActionName)
		}
		c.unitTag = names.NewUnitTag(unitName)
		c.actionName = ActionName
		if len(args) == 2 {
			return nil
		}
		// Parse CLI key-value args if they exist.
		c.args = make([][]string, 0)
		for _, arg := range args[2:] {
			thisArg := strings.SplitN(arg, "=", 2)
			if len(thisArg) != 2 {
				return errors.Errorf("argument %q must be of the form key...=value", arg)
			}
			keySlice := strings.Split(thisArg[0], ".")
			// check each key for validity
			for _, key := range keySlice {
				if valid := keyRule.MatchString(key); !valid {
					return errors.Errorf("key %q must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric and hyphens", key)
				}
			}
			// c.args={..., [key, key, key, key, value]}
			c.args = append(c.args, append(keySlice, thisArg[1]))
		}
		return nil
	}
}

func (c *runCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	actionParams := map[string]interface{}{}

	if c.paramsYAML.Path != "" {
		b, err := c.paramsYAML.Read(ctx)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(b, &actionParams)
		if err != nil {
			return err
		}

		conformantParams, err := common.ConformYAML(actionParams)
		if err != nil {
			return err
		}

		betterParams, ok := conformantParams.(map[string]interface{})
		if !ok {
			return errors.New("params must contain a YAML map with string keys")
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
				return err
			}
		}
		// Insert the value in the map.
		addValueToMap(keys, cleansedValue, actionParams)
	}

	conformantParams, err := common.ConformYAML(actionParams)
	if err != nil {
		return err
	}

	typedConformantParams, ok := conformantParams.(map[string]interface{})
	if !ok {
		return errors.Errorf("params must be a map, got %T", typedConformantParams)
	}

	actionParam := params.Actions{
		Actions: []params.Action{{
			Receiver:   c.unitTag.String(),
			Name:       c.actionName,
			Parameters: actionParams,
		}},
	}

	results, err := api.Enqueue(actionParam)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return errors.New("illegal number of results returned")
	}

	result := results.Results[0]

	if result.Error != nil {
		return result.Error
	}

	if result.Action == nil {
		return errors.New("action failed to enqueue")
	}

	tag, err := names.ParseActionTag(result.Action.Tag)
	if err != nil {
		return err
	}

	output := map[string]string{"Action queued with id": tag.Id()}
	return c.out.Write(ctx, output)
}
