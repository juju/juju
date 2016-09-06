// Copyright 2013 Canonical Ltd.
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

See also:
    models
    set-model-config
    unset-model-config
`

func (c *getCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-model-config",
		Aliases: []string{"model-config"},
		Args:    "[<model key>]",
		Purpose: "Displays configuration settings for a model.",
		Doc:     getModelHelpDoc,
	}
}

func (c *getCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
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

func (c *getCommand) isModelAttrbute(attr string) bool {
	switch attr {
	case config.NameKey, config.TypeKey, config.UUIDKey:
		return true
	}
	return false
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

	for attrName := range attrs {
		// We don't want model attributes included, these are available
		// via show-model.
		if c.isModelAttrbute(attrName) {
			delete(attrs, attrName)
		}
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			err := cmd.FormatYaml(ctx.Stdout, value.Value)
			if err != nil {
				return err
			}
			return nil
		}
		return errors.Errorf("key %q not found in %q model.", c.key, attrs["name"])
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

// formatConfigTabular writes a tabular summary of config information.
func formatConfigTabular(writer io.Writer, value interface{}) error {
	configValues, ok := value.(config.ConfigValues)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", configValues, value)
	}

	tw := output.TabWriter(writer)
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
		out := &bytes.Buffer{}
		err := cmd.FormatYaml(out, info.Value)
		if err != nil {
			return errors.Annotatef(err, "formatting value for %q", name)
		}
		// Some attribute values have a newline appended
		// which makes the output messy.
		valString := strings.TrimSuffix(out.String(), "\n")
		p(name, info.Source, valString)
	}

	tw.Flush()
	return nil
}
