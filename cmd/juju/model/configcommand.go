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

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/environs/config"
)

const (
	modelConfigSummary = "Displays or sets configuration values on a model."
	modelConfigHelpDoc = `
By default, all configuration (keys, source, and values) for the current model
are displayed.

Supplying one key name returns only the value for the key. Supplying key=value
will set the supplied key to the supplied value, this can be repeated for
multiple keys.

Examples
    juju model-config default-series
    juju model-config -m mycontroller:mymodel
    juju model-config ftp-proxy=10.0.0.1:8000
    juju model-config -m othercontroller:mymodel default-series=yakkety test-mode=false
    juju model-config --reset default-series test-mode

See also:
    models
    model-defaults
`
)

// NewConfigCommand wraps configCommand with sane model settings.
func NewConfigCommand() cmd.Command {
	return modelcmd.Wrap(&configCommand{})
}

type attributes map[string]interface{}

// configCommand is the simplified command for accessing and setting
// attributes related to model configuration.
type configCommand struct {
	api configCommandAPI
	modelcmd.ModelCommandBase
	out cmd.Output

	action func(configCommandAPI, *cmd.Context) error // The action which we want to handle, set in cmd.Init.
	keys   []string
	reset  bool // Flag denoting whether we are resetting the keys provided.
	values attributes
}

// Info implements part of the cmd.Command interface.
func (c *configCommand) Info() *cmd.Info {
	return &cmd.Info{
		Args:    "[<model-key>[<=value>] ...]",
		Doc:     modelConfigHelpDoc,
		Name:    "model-config",
		Purpose: modelConfigSummary,
	}
}

// SetFlags implements part of the cmd.Command interface.
func (c *configCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"tabular": formatConfigTabular,
		"yaml":    cmd.FormatYaml,
	})
	f.BoolVar(&c.reset, "reset", false, "Reset the provided keys to be empty")
}

// Init implements part of the cmd.Command interface.
func (c *configCommand) Init(args []string) error {
	if c.reset {
		// We're doing resetConfig.
		if len(args) == 0 {
			return errors.New("no keys specified")
		}
		for _, k := range args {
			if k == config.AgentVersionKey {
				return errors.Errorf("agent-version cannot be reset")
			}
		}
		c.keys = args
		c.action = c.resetConfig
		return nil
	}

	if len(args) > 0 && strings.Contains(args[0], "=") {
		// We're setting values.
		options, err := keyvalues.Parse(args, true)
		if err != nil {
			return errors.Trace(err)
		}
		c.values = make(attributes)
		for k, v := range options {
			if k == config.AgentVersionKey {
				return errors.Errorf(`agent-version must be set via "upgrade-juju"`)
			}
			c.values[k] = v
		}

		c.action = c.setConfig
		return nil
	}

	val, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.New("can only retrieve a single value, or all values")
	}

	// We're doing getConfig.
	if val != "" {
		c.keys = []string{val}
	}
	c.action = c.getConfig
	return nil
}

// configCommandAPI defines an API interface to be used during testing.
type configCommandAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
	ModelGetWithMetadata() (config.ConfigValues, error)
	ModelSet(config map[string]interface{}) error
	ModelUnset(keys ...string) error
}

// isModelAttribute returns if the supplied attribute is a valid model
// attribute.
func (c *configCommand) isModelAttrbute(attr string) bool {
	switch attr {
	case config.NameKey, config.TypeKey, config.UUIDKey:
		return true
	}
	return false
}

// getAPI returns the API. This allows passing in a test configCommandAPI
// implementation.
func (c *configCommand) getAPI() (configCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := modelconfig.NewClient(api)
	return client, nil
}

// Run implements the meaty part of the cmd.Command interface.
func (c *configCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return c.action(client, ctx)
}

// reset unsets the keys provided to the command.
func (c *configCommand) resetConfig(client configCommandAPI, ctx *cmd.Context) error {
	// ctx unused in this method

	// extra call to the API to retrieve env config
	envAttrs, err := client.ModelGet()
	if err != nil {
		return err
	}
	for _, key := range c.keys {
		// check if the key exists in the existing env config
		// and warn the user if the key is not defined in
		// the existing config
		if _, exists := envAttrs[key]; !exists {
			// TODO(ro) This error used to be a false positive. Now, if it is
			// printed, there really is a problem or misspelling. Ian would like to
			// do some further testing and look at making this situation a fatal
			// error, not just a warning. I think it's ok to leave for now, but
			// with a todo.
			logger.Warningf("key %q is not defined in the current model configuration: possible misspelling", key)
		}

	}
	return block.ProcessBlockedError(client.ModelUnset(c.keys...), block.BlockChange)
}

// set sets the provided key/value pairs on the model.
func (c *configCommand) setConfig(client configCommandAPI, ctx *cmd.Context) error {
	// ctx unused in this method.
	envAttrs, err := client.ModelGet()
	if err != nil {
		return err
	}
	for key := range c.values {
		if _, exists := envAttrs[key]; !exists {
			logger.Warningf("key %q is not defined in the current model configuration: possible misspelling", key)
		}

	}
	return block.ProcessBlockedError(client.ModelSet(c.values), block.BlockChange)
}

// get writes the value of a single key or the full output for the model to the cmd.Context.
func (c *configCommand) getConfig(client configCommandAPI, ctx *cmd.Context) error {
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

	if len(c.keys) == 1 {
		key := c.keys[0]
		if value, found := attrs[key]; found {
			if c.out.Name() == "tabular" {
				return cmd.FormatYaml(ctx.Stdout, value.Value)
			}
			attrs = config.ConfigValues{
				key: config.ConfigValue{
					Source: value.Source,
					Value:  value.Value,
				},
			}
		} else {
			return errors.Errorf("key %q not found in %q model.", key, attrs["name"])
		}
	}
	return c.out.Write(ctx, attrs)
}

// formatConfigTabular writes a tabular summary of config information.
func formatConfigTabular(writer io.Writer, value interface{}) error {
	configValues, ok := value.(config.ConfigValues)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", configValues, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	var valueNames []string
	for name := range configValues {
		valueNames = append(valueNames, name)
	}
	sort.Strings(valueNames)
	w.Println("ATTRIBUTE", "FROM", "VALUE")

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
		w.Println(name, info.Source, valString)
	}

	tw.Flush()
	return nil
}
