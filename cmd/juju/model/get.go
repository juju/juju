// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
)

func NewGetCommand() cmd.Command {
	return modelcmd.Wrap(&getCommand{})
}

// getCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type getCommand struct {
	modelcmd.ModelCommandBase
	api GetModelAPI
	key string
	out cmd.Output
}

const getModelHelpDoc = `
By default, all configuration (keys and values) for the model are
displayed if a key is not specified.
By default, the model is the current model.

Examples:

    juju get-model-config default-series
    juju get-model-config -m mymodel type

See also: models
          set-model-config
          unset-model-config
`

func (c *getCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-model-config",
		Aliases: []string{"model-config"},
		Args:    "[<model key>]",
		Purpose: "Displays configuration settings for a model.",
		Doc:     strings.TrimSpace(getModelHelpDoc),
	}
}

func (c *getCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatConfigTabular,
	})
}

func (c *getCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

type GetModelAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
	ModelGetWithMetadata() (config.ConfigValues, error)
}

func (c *getCommand) getAPI() (GetModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return modelconfig.NewClient(api), nil
}

func (c *getCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.ModelGetWithMetadata()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			out, err := cmd.FormatSmart(value.Value)
			if err != nil {
				return err
			}
			fmt.Fprintf(ctx.Stdout, "%v\n", string(out))
			return nil
		}
		return fmt.Errorf("key %q not found in %q model.", c.key, attrs["name"])
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

// formatConfigTabular returns a tabular summary of config information.
func formatConfigTabular(value interface{}) ([]byte, error) {
	configValues, ok := value.(config.ConfigValues)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", configValues, value)
	}

	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	p := func(values ...string) {
		text := strings.Join(values, "\t")
		fmt.Fprintln(tw, text)
	}
	var valueNames []string
	for name := range configValues {
		valueNames = append(valueNames, name)
	}
	sort.Strings(valueNames)
	p("ATTRIBUTE\tFROM\tVALUE")

	for _, name := range valueNames {
		info := configValues[name]
		val, err := cmd.FormatSmart(info.Value)
		if err != nil {
			return nil, errors.Annotatef(err, "formatting value for %q", name)
		}
		// Some attribute values have a newline appended
		// which makes the output messy.
		valString := strings.TrimSuffix(string(val), "\n")
		p(name, info.Source, valString)
	}

	tw.Flush()
	return out.Bytes(), nil
}
