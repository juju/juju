// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
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
		Doc:     modelDefaultsHelpDoc,
	}
}

func (c *getDefaultsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
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
	ModelDefaults() (config.ModelDefaultAttributes, error)
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
			attrs = config.ModelDefaultAttributes{
				c.key: value,
			}
		} else {
			return errors.Errorf("key %q not found in %q model defaults.", c.key, attrs["name"])
		}
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

// formatConfigTabular writes a tabular summary of default config information.
func formatDefaultConfigTabular(writer io.Writer, value interface{}) error {
	defaultValues, ok := value.(config.ModelDefaultAttributes)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", defaultValues, value)
	}

	tw := output.TabWriter(writer)

	ph := func(values ...string) {
		text := strings.Join(values, "\t")
		fmt.Fprintln(tw, text)
	}

	p := func(name string, value config.AttributeDefaultValues) {
		var c, d interface{}
		switch value.Default {
		case nil:
			d = "-"
		case "":
			d = `""`
		default:
			d = value.Default
		}
		switch value.Controller {
		case nil:
			c = "-"
		case "":
			c = `""`
		default:
			c = value.Controller
		}
		row := fmt.Sprintf("%s\t%v\t%v", name, d, c)
		fmt.Fprintln(tw, row)
		for _, region := range value.Regions {
			row := fmt.Sprintf("  %s\t%v\t-", region.Name, region.Value)
			fmt.Fprintln(tw, row)
		}
	}
	var valueNames []string
	for name := range defaultValues {
		valueNames = append(valueNames, name)
	}
	sort.Strings(valueNames)
	ph("ATTRIBUTE\tDEFAULT\tCONTROLLER")

	for _, name := range valueNames {
		info := defaultValues[name]
		out := &bytes.Buffer{}
		err := cmd.FormatYaml(out, info)
		if err != nil {
			return errors.Annotatef(err, "formatting value for %q", name)
		}
		p(name, info)
	}

	tw.Flush()
	return nil
}
