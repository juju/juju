// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/names/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/feature"
)

type showCommand struct {
	ActionCommandBase

	applicationTag names.ApplicationTag
	actionName     string

	out cmd.Output
}

var showActionDoc = `
Show detailed information about an action on the target application.

Examples:
    juju show-action postgresql backup

See also:
    list-actions
    run-action
`

// NewShowCommand returns a command to print action information.
func NewShowCommand() cmd.Command {
	return modelcmd.Wrap(&showCommand{})
}

func (c *showCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no application specified")
	case 1:
		return errors.New("no action specified")
	case 2:
		appName := args[0]
		if !names.IsValidApplication(appName) {
			return errors.Errorf("invalid application name %q", appName)
		}
		c.applicationTag = names.NewApplicationTag(appName)
		c.actionName = args[1]
		return nil
	default:
		return cmd.CheckEmpty(args[2:])
	}
}

func (c *showCommand) Info() *cmd.Info {
	info := jujucmd.Info(&cmd.Info{
		Name:    "show-action",
		Args:    "<application> <action>",
		Purpose: "Shows detailed information about an action.",
		Doc:     showActionDoc,
	})
	if featureflag.Enabled(feature.ActionsV2) {
		info.Doc = strings.Replace(info.Doc, "run-action", "run", -1)
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
	info, ok := actions[c.actionName]
	if !ok {
		ctx.Infof("unknown action %q\n", c.actionName)
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
