// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"regexp"

	yaml "gopkg.in/yaml.v1"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"
	"launchpad.net/gnuflag"
)

// DoCommand enqueues an Action for running on the given unit with given
// params
type DoCommand struct {
	ActionCommandBase
	unitTag    names.UnitTag
	actionName string
	paramsYAML cmd.FileVar
	async      bool
	out        cmd.Output
	undefinedActionCommand
}

const doDoc = `
Queue an Action for execution on a given unit, with a given set of params.
Displays the ID of the Action for use with 'juju kill', 'juju status', etc.
The command will wait until it receives a result unless --async is used.

Params are validated according to the charm for the unit's service.  The 
valid params can be seen using "juju action defined <service>".  Params must
be in a yaml file which is passed with the --params flag.

Examples:

$ juju do mysql/2 pause

finished

$ juju do mysql/3 backup --async
action: <UUID>

$ juju status <UUID>
result:
  status: success
  file:
    size: 873.2
    units: GB
    name: foo.sql

$ juju do mysql/3 backup --async --params parameters.yml
...
`

// SetFlags offers an option for YAML output.
func (c *DoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(&c.paramsYAML, "params", "path to yaml-formatted params file")
	f.BoolVar(&c.async, "async", false, "run in the background")
}

func (c *DoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "do",
		Args:    "<unit> <action name>",
		Purpose: "WIP: queue an action for execution",
		Doc:     doDoc,
	}
}

// Init gets the unit tag, and checks for other correct args.
func (c *DoCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no unit specified")
	case 1:
		return errors.New("no action specified")
	case 2:
		unitName := args[0]
		if !names.IsValidUnit(unitName) {
			return errors.Errorf("invalid unit name %q", unitName)
		}
		actionName := args[1]
		actionNameRule := regexp.MustCompile("^[a-z](?:[a-z-]*[a-z])?$")
		if valid := actionNameRule.MatchString(actionName); !valid {
			return fmt.Errorf("invalid action name %q", actionName)
		}
		c.unitTag = names.NewUnitTag(unitName)
		c.actionName = actionName

		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

func (c *DoCommand) Run(ctx *cmd.Context) error {
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

		conformantParams, err := conform(actionParams)
		if err != nil {
			return err
		}

		betterParams, ok := conformantParams.(map[string]interface{})
		if !ok {
			return errors.New("params must contain a YAML map with string keys")
		}

		actionParams = betterParams
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
		return errors.New("only one result must be received")
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

// err = c.out.Write(ctx, map[string]string{"Action queued with id": tag})
// if err != nil {
// 	return err
// }

// for _ = range time.Tick(1 * time.Second) {
// 	completed, err := api.ListCompleted(params.Entities{
// 		Entities: []params.Entity{{c.unitTag.String()}},
// 	})
// 	if err != nil {
// 		return err
// 	}

// 	if len(completed.Actions) != 1 {
// 		return errors.New("only one result must be received")
// 	}
// 	err = completed.Actions[0].Error
// 	if err != nil {
// 		return err
// 	}

// 	results := completed.Actions[0].Actions
// 	if len(results) == 0 {
// 		continue
// 	}
// 	if len(results) > 1 {
// 		return errors.New("too many action results")
// 	}

// 	err = displayActionResult(results[0], ctx, c.out)
// 	if err != nil {
// 		return err
// 	}
// }

// return nil
//}
