// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/feature"
)

type showCommand struct {
	ActionCommandBase

	applicationTag names.ApplicationTag
	functionName   string

	out cmd.Output
}

var showActionDoc = `
Show detailed information about a function on the target application.

Examples:
    juju show-function postgresql backup

See also:
    call
    list-functions
`

// NewShowCommand returns a command to print function information.
func NewShowCommand() cmd.Command {
	return modelcmd.Wrap(&showCommand{})
}

func (c *showCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no application name specified")
	case 1:
		return errors.New("no function name specified")
	case 2:
		appName := args[0]
		if !names.IsValidApplication(appName) {
			return errors.Errorf("invalid application name %q", appName)
		}
		c.applicationTag = names.NewApplicationTag(appName)
		c.functionName = args[1]
		return nil
	default:
		return cmd.CheckEmpty(args[2:])
	}
}

func (c *showCommand) Info() *cmd.Info {
	info := jujucmd.Info(&cmd.Info{
		Name:    "show-function",
		Args:    "<application name> <function name>",
		Purpose: "Shows detailed information about a function.",
		Doc:     showActionDoc,
		Aliases: []string{"show-action"},
	})
	if featureflag.Enabled(feature.JujuV3) {
		info.Aliases = nil
	}
	return info
}

func (c *showCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	actions, err := api.ApplicationCharmActions(params.Entity{Tag: c.applicationTag.String()})
	if err != nil {
		return err
	}
	info, ok := actions[c.functionName]
	if !ok {
		ctx.Infof("unknown function %q\n", c.functionName)
		return cmd.ErrSilent
	}

	fmt.Fprintln(ctx.Stdout, info.Description+"\n\nArguments")

	args := make(map[string]actionArg)
	properties, ok := info.Params["properties"].(map[string]interface{})
	if ok {
		for argName, info := range properties {
			infoMap, ok := info.(map[string]interface{})
			if !ok {
				continue
			}
			args[argName] = actionArg{
				Type:        infoMap["type"],
				Description: infoMap["description"],
			}
		}
	}

	argInfo, err := yaml.Marshal(args)
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintln(ctx.Stdout, string(argInfo))
	return nil
}

type actionArg struct {
	// Use a struct so we can control the order of the printed values.
	Type        interface{} `yaml:"type"`
	Description interface{} `yaml:"description"`
}
