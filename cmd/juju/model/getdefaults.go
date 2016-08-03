// Copyright 2016 Canonical Ltd.
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

// NewModelDefaultsCommand returns a command used to print the
// default model config attributes.
func NewModelDefaultsCommand() cmd.Command {
	c := &getDefaultsCommand{}
	c.newAPIFunc = func() (modelDefaultsAPI, error) {
		api, err := c.NewAPIRoot()
		if err != nil {
			return nil, errors.Annotate(err, "opening API connection")
		}
		return modelconfig.NewClient(api), nil
	}
	return modelcmd.Wrap(c)
}

type getDefaultsCommand struct {
	modelcmd.ModelCommandBase
	newAPIFunc func() (modelDefaultsAPI, error)
	key        string
	out        cmd.Output
}

const modelDefaultsHelpDoc = `
By default, all default configuration (keys and values) are
displayed if a key is not specified.
By default, the model is the current model.

Examples:

    juju model-defaults
    juju model-defaults http-proxy
    juju model-defaults -m mymodel type

See also:
    models
    set-model-defaults
    unset-model-defaults
    set-model-config
    get-model-config
    unset-model-config
`

func (c *getDefaultsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "model-defaults",
		Args:    "[<model key>]",
		Purpose: "Displays default configuration settings for a model.",
		Doc:     strings.TrimSpace(modelDefaultsHelpDoc),
	}
}

func (c *getDefaultsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatDefaultConfigTabular,
	})
}

func (c *getDefaultsCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

// modelDefaultsAPI defines the api methods used by this command.
type modelDefaultsAPI interface {
	// Close closes the api connection.
	Close() error

	// ModelDefaults returns the default config values used when creating a new model.
	ModelDefaults() (config.ConfigValues, error)
}

func (c *getDefaultsCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.ModelDefaults()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, ok := attrs[c.key]; ok {
			attrs = config.ConfigValues{
				c.key: value,
			}
		} else {
			return errors.Errorf("key %q not found in %q model defaults.", c.key, attrs["name"])
		}
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

// formatConfigTabular returns a tabular summary of default config information.
func formatDefaultConfigTabular(value interface{}) ([]byte, error) {
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
	p("ATTRIBUTE\tDEFAULT\tCONTROLLER")

	for _, name := range valueNames {
		info := configValues[name]
		val, err := cmd.FormatSmart(info.Value)
		if err != nil {
			return nil, errors.Annotatef(err, "formatting value for %q", name)
		}
		d := "-"
		c := "-"
		if info.Source == "default" {
			d = string(val)
		} else {
			c = string(val)
		}
		p(name, d, c)
	}

	tw.Flush()
	return out.Bytes(), nil
}
