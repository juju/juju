// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model

import (
	"bytes"
	"io"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/keyvalues"

	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/environs/config"
)

const (
	modelDefaultsSummary = `Displays or sets default configuration settings for a model.`
	modelDefaultsHelpDoc = `
By default, all default configuration (keys and values) are
displayed if a key is not specified. Supplying key=value will set the
supplied key to the supplied value. This can be repeated for multiple keys.
By default, the model is the current model.


Examples:
    juju model-defaults
    juju model-defaults http-proxy
    juju model-defaults -m mymodel type
    juju model-defaults ftp-proxy=10.0.0.1:8000
    juju model-defaults -m othercontroller:mymodel default-series=yakkety test-mode=false
    juju model-defaults --reset default-series test-mode

See also:
    models
    model-config
`
)

// NewDefaultsCommand wraps defaultsCommand with sane model settings.
func NewDefaultsCommand() cmd.Command {
	return modelcmd.WrapController(&defaultsCommand{})
}

// defaultsCommand is compound command for accessing and setting attributes
// related to default model configuration.
type defaultsCommand struct {
	modelcmd.ControllerCommandBase
	api defaultsCommandAPI
	out cmd.Output

	action func(defaultsCommandAPI, *cmd.Context) error // The function handling the input, set in Init.
	keys   []string
	reset  bool // Flag indicating if we are resetting the keys provided.
	values attributes
}

// defaultsCommandAPI defines an API to be used during testing.
type defaultsCommandAPI interface {
	// Close closes the api connection.
	Close() error

	// ModelDefaults returns the default config values used when creating a new model.
	ModelDefaults() (config.ModelDefaultAttributes, error)

	// SetModelDefaults sets the default config values to use
	// when creating new models.
	SetModelDefaults(cloud, region string, config map[string]interface{}) error

	// UnsetModelDefaults clears the default model
	// configuration values.
	UnsetModelDefaults(cloud, region string, keys ...string) error
}

// Info implements part of the cmd.Command interface.
func (c *defaultsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Args:    "[<model-key>[<=value>] ...]",
		Doc:     modelDefaultsHelpDoc,
		Name:    "model-defaults",
		Purpose: modelDefaultsSummary,
	}
}

// SetFlags implements part of the cmd.Command interface.
func (c *defaultsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatDefaultConfigTabular,
	})
	f.BoolVar(&c.reset, "reset", false, "Reset the provided keys to be empty")
}

// Init implements part of the cmd.Command interface.
func (c *defaultsCommand) Init(args []string) error {
	if c.reset {
		// We're resetting defaults.
		if len(args) == 0 {
			return errors.New("no keys specified")
		}
		for _, k := range args {
			if k == config.AgentVersionKey {
				return errors.Errorf("%q cannot be reset", config.AgentVersionKey)
			}
		}
		c.keys = args

		c.action = c.resetDefaults
		return nil
	}

	if len(args) > 0 && strings.Contains(args[0], "=") {
		// We're setting defaults.
		options, err := keyvalues.Parse(args, true)
		if err != nil {
			return errors.Trace(err)
		}
		c.values = make(attributes)
		for k, v := range options {
			if k == config.AgentVersionKey {
				return errors.Errorf(`%q must be set via "upgrade-juju"`, config.AgentVersionKey)
			}
			c.values[k] = v
		}

		c.action = c.setDefaults
		return nil

	}
	// We're getting defaults.
	val, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.New("can only retrieve a single value, or all values")
	}
	if val != "" {
		c.keys = []string{val}
	}
	c.action = c.getDefaults
	return nil
}

// getAPI sets the api on the command. This allows passing in a test
// ModelDefaultsAPI implementation.
func (c *defaultsCommand) getAPI() (defaultsCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}

	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := modelmanager.NewClient(api)

	return client, nil
}

// Run implements part of the cmd.Command interface.
func (c *defaultsCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	return c.action(client, ctx)
}

func (c *defaultsCommand) getDefaults(client defaultsCommandAPI, ctx *cmd.Context) error {
	attrs, err := client.ModelDefaults()
	if err != nil {
		return err
	}

	if len(c.keys) == 1 {
		key := c.keys[0]
		if value, ok := attrs[key]; ok {
			attrs = config.ModelDefaultAttributes{
				key: value,
			}
		} else {
			return errors.Errorf("key %q not found in %q model defaults.", key, attrs["name"])
		}
	}
	// If c.keys is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

func (c *defaultsCommand) setDefaults(client defaultsCommandAPI, ctx *cmd.Context) error {
	// ctx unused in this method.
	if err := c.verifyKnownKeys(client); err != nil {
		return errors.Trace(err)
	}
	// TODO(wallyworld) - call with cloud and region when that bit is done
	return block.ProcessBlockedError(client.SetModelDefaults("", "", c.values), block.BlockChange)
}

func (c *defaultsCommand) resetDefaults(client defaultsCommandAPI, ctx *cmd.Context) error {
	// ctx unused in this method.
	if err := c.verifyKnownKeys(client); err != nil {
		return errors.Trace(err)
	}
	// TODO(wallyworld) - call with cloud and region when that bit is done
	return block.ProcessBlockedError(client.UnsetModelDefaults("", "", c.keys...), block.BlockChange)

}

// verifyKnownKeys is a helper to validate the keys we are operating with
// against the set of known attributes from the model.
func (c *defaultsCommand) verifyKnownKeys(client defaultsCommandAPI) error {
	known, err := client.ModelDefaults()
	if err != nil {
		return errors.Trace(err)
	}
	keys := func() []string {
		if c.keys != nil {
			return c.keys
		}
		keys := []string{}
		for k, _ := range c.values {
			keys = append(keys, k)
		}
		return keys
	}
	for _, key := range keys() {
		// check if the key exists in the known config
		// and warn the user if the key is not defined
		if _, exists := known[key]; !exists {
			logger.Warningf(
				"key %q is not defined in the known model configuration: possible misspelling", key)
		}
	}
	return nil
}

// formatConfigTabular writes a tabular summary of default config information.
func formatDefaultConfigTabular(writer io.Writer, value interface{}) error {
	defaultValues, ok := value.(config.ModelDefaultAttributes)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", defaultValues, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

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
		w.Println(name, d, c)
		for _, region := range value.Regions {
			w.Println("  "+region.Name, region.Value, "-")
		}
	}
	var valueNames []string
	for name := range defaultValues {
		valueNames = append(valueNames, name)
	}
	sort.Strings(valueNames)

	w.Println("ATTRIBUTE", "DEFAULT", "CONTROLLER")

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
