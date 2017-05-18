// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/set"

	apicontroller "github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/controller"
)

func NewGetConfigCommand() cmd.Command {
	return modelcmd.WrapController(&getConfigCommand{})
}

// getConfigCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type getConfigCommand struct {
	modelcmd.ControllerCommandBase
	api controllerAPI
	key string
	out cmd.Output
}

const getControllerHelpDoc = `
By default, all configuration (keys and values) for the controller are
displayed if a key is not specified.

Examples:

    juju controller-config
    juju controller-config api-port
    juju controller-config -c mycontroller

See also:
    controllers
`

func (c *getConfigCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "controller-config",
		Args:    "[<attribute key>]",
		Purpose: "Displays configuration settings for a controller.",
		Doc:     strings.TrimSpace(getControllerHelpDoc),
	}
}

func (c *getConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"tabular": formatConfigTabular,
		"yaml":    cmd.FormatYaml,
	})
}

func (c *getConfigCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

type controllerAPI interface {
	Close() error
	ControllerConfig() (controller.Config, error)
}

func (c *getConfigCommand) getAPI() (controllerAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicontroller.NewClient(root), nil
}

func (c *getConfigCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.ControllerConfig()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			if c.out.Name() == "tabular" {
				// The user has not specified that they want
				// YAML or JSON formatting, so we print out
				// the value unadorned.
				return c.out.WriteFormatter(ctx, cmd.FormatSmart, value)
			}
			return c.out.Write(ctx, value)
		}
		return errors.Errorf("key %q not found in %q controller.", c.key, controllerName)
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

func formatConfigTabular(writer io.Writer, value interface{}) error {
	controllerConfig, ok := value.(controller.Config)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", controllerConfig, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	valueNames := make(set.Strings)
	for name := range controllerConfig {
		valueNames.Add(name)
	}
	w.Println("Attribute", "Value")

	for _, name := range valueNames.SortedValues() {
		value := controllerConfig[name]

		var out bytes.Buffer
		err := cmd.FormatYaml(&out, value)
		if err != nil {
			return errors.Annotatef(err, "formatting value for %q", name)
		}
		// Some attribute values have a newline appended
		// which makes the output messy.
		valString := strings.TrimSuffix(out.String(), "\n")
		w.Println(name, valString)
	}

	w.Flush()
	return nil
}
