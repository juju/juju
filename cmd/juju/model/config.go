// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/config"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	envconfig "github.com/juju/juju/environs/config"
)

const (
	modelConfigSummary        = "Displays or sets configuration values on a model."
	modelConfigHelpDocPartOne = `
To view all configuration values for the current model:

    juju model-config

You can target a specific model using the ` + "`-m`" + ` flag:

    juju model-config -m <model>
    juju model-config -m <controller>:<model>

	By default, the config will be printed in a tabular format. You can instead
print it in the ` + "`json`" + ` or ` + "`yaml`" + ` format using the ` + "`--format`" + ` flag:

    juju model-config --format json
    juju model-config --format yaml

To view the value of a single config key:

    juju model-config key

To set config values:

    juju model-config key1=val1 key2=val2 ...

You can also reset config keys to their default values:

    juju model-config --reset key1
    juju model-config --reset key1,key2,key3

You may simultaneously set some keys and reset others:

    juju model-config key1=val1 key2=val2 --reset key3,key4

Config values can be imported from a yaml file using the ` + "`--file`" + ` flag:

    juju model-config --file=path/to/cfg.yaml

This allows you to, e.g., save a model's config to a file:

    juju model-config --format=yaml > cfg.yaml

and then import the config later. Note that the output of ` + "`model-config`" + `
may include read-only values, which will cause an error when importing later.
To prevent the error, use the ` + "`--ignore-read-only-fields`" + ` flag:

    juju model-config --file=cfg.yaml --ignore-read-only-fields

You can also read from ` + "`stdin`" + ` using ` + "`-`" + `, which allows you to pipe config values
from one model to another:

    juju model-config -c c1 --format=yaml \
      | juju model-config -c c2 --file=- --ignore-read-only-fields

You can simultaneously read config from a yaml file and set config keys
as above. The command-line args will override any values specified in the file.

The ` + "`default-series`" + ` key is deprecated in favour of ` + "`default-base`" + `. For example:
` + "`default-base=ubuntu@22.04`" + `.
`
	modelConfigHelpDocKeys = `
The following keys are available:
`
	modelConfigHelpDocPartTwo = `
`
	modelConfigExamples = `
Print the value of default-base:

    juju model-config default-base

Print the model config of model ` + "`mycontroller:mymodel`" + `:

    juju model-config -m mycontroller:mymodel

Set the value of ftp-proxy to ` + "`10.0.0.1:8000`" + `:

    juju model-config ftp-proxy=10.0.0.1:8000

Set the model config to key=value pairs defined in a file:

    juju model-config --file path/to/file.yaml

Set model config values of a specific model:

    juju model-config -m othercontroller:mymodel default-base=ubuntu@22.04 test-mode=false

Reset the values of the provided keys to model defaults:

    juju model-config --reset default-base,test-mode
`
)

var modelConfigBase = config.ConfigCommandBase{
	Resettable: true,
	CantReset:  []string{envconfig.AgentVersionKey, envconfig.CharmHubURLKey},
}

// NewConfigCommand wraps configCommand with sane model settings.
func NewConfigCommand() cmd.Command {
	return modelcmd.Wrap(&configCommand{configBase: modelConfigBase})
}

type configAttrs map[string]interface{}

// CoerceFormat attempts to convert the configAttrs values from the complex type
// to the more simple type. This is because the output of this command outputs
// in the following format:
//
//	resource-name:
//	   value: foo
//	   source: default
//
// Where the consuming side of the command expects it in the following format:
//
//	resource-name: foo
//
// CoerceFormat attempts to diagnose this and attempt to do this correctly.
func (a configAttrs) CoerceFormat() (configAttrs, error) {
	coerced := make(map[string]interface{})

	fields := schema.FieldMap(schema.Fields{
		"value":  schema.Any(),
		"source": schema.String(),
	}, nil)

	for k, v := range a {
		out, err := fields.Coerce(v, []string{})
		if err != nil {
			// Fallback to the old format and just pass through the value.
			coerced[k] = v
			continue
		}

		m := out.(map[string]interface{})
		v = m["value"]

		// Resource tags in the new output format is a map[string]interface{},
		// but it should be of the format `foo=bar baz=boo`.
		if k == "resource-tags" {
			tags, err := coerceResourceTags(v)
			if err != nil {
				return nil, errors.Annotate(err, "unable to read resource-tags")
			}
			v = tags
		}

		coerced[k] = v
	}

	return coerced, nil
}

func coerceResourceTags(resourceTags interface{}) (string, error) {
	// When coercing a resource tag, the tags in question might already be in
	// the correct format of a string. If that's the case, we should pass on
	// doing the coercion.
	if tags, ok := resourceTags.(string); ok {
		return tags, nil
	}

	// It's not what we expect the resourceTags to be, so try and coerce the
	// tags from a string to a map[string]interface{} and put it back into a
	// format that we can consume.
	tags := schema.StringMap(schema.Any())
	out, err := tags.Coerce(resourceTags, []string{})
	if err != nil {
		return "", errors.Trace(err)
	}

	m := out.(map[string]interface{})
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, fmt.Sprintf("%s=%v", k, v))
	}

	return strings.Join(result, " "), nil
}

// configCommand is the simplified command for accessing and setting
// attributes related to model configuration.
type configCommand struct {
	modelcmd.ModelCommandBase
	configBase config.ConfigCommandBase
	api        configCommandAPI
	out        cmd.Output

	// Extra `model-config`-specific fields
	ignoreReadOnlyFields bool
}

// configCommandAPI defines an API interface to be used during testing.
type configCommandAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
	ModelGetWithMetadata() (envconfig.ConfigValues, error)
	ModelSet(config map[string]interface{}) error
	ModelUnset(keys ...string) error
}

// Info implements part of the cmd.Command interface.
func (c *configCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Args:     "[<model-key>[=<value>] ...]",
		Name:     "model-config",
		Purpose:  modelConfigSummary,
		Examples: modelConfigExamples,
		SeeAlso: []string{
			"models",
			"model-defaults",
			"show-cloud",
			"controller-config",
		},
	}
	if details, err := ConfigDetails(); err == nil {
		if formattedDetails, err := common.FormatConfigSchema(details); err == nil {
			info.Doc = fmt.Sprintf("%s%s\n%s%s",
				modelConfigHelpDocPartOne,
				modelConfigHelpDocKeys,
				formattedDetails,
				modelConfigHelpDocPartTwo)
			return jujucmd.Info(info)
		}
	}
	info.Doc = fmt.Sprintf("%s%s",
		modelConfigHelpDocPartOne,
		modelConfigHelpDocPartTwo)
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *configCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.configBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"tabular": formatConfigTabular,
		"yaml":    cmd.FormatYaml,
	})
	f.BoolVar(&c.ignoreReadOnlyFields, "ignore-read-only-fields", false, "Ignore read only fields that might cause errors to be emitted while processing yaml documents")
}

// Init implements part of the cmd.Command interface.
func (c *configCommand) Init(args []string) error {
	return c.configBase.Init(args)
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

	for _, action := range c.configBase.Actions {
		var err error
		switch action {
		case config.GetOne:
			err = c.getConfig(client, ctx)
		case config.SetArgs:
			err = c.setConfig(client, c.configBase.ValsToSet)
		case config.SetFile:
			var attrs config.Attrs
			attrs, err = c.configBase.ReadFile(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			err = c.setConfig(client, attrs)
		case config.Reset:
			err = c.resetConfig(client)
		default:
			err = c.getAllConfig(client, ctx)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// resetConfig unsets the keys provided to the command.
func (c *configCommand) resetConfig(client configCommandAPI) error {
	// ctx unused in this method
	if err := c.verifyKnownKeys(client, c.configBase.KeysToReset); err != nil {
		return errors.Trace(err)
	}

	return block.ProcessBlockedError(client.ModelUnset(c.configBase.KeysToReset...), block.BlockChange)
}

// setConfig sets the provided key/value pairs on the model.
func (c *configCommand) setConfig(client configCommandAPI, attrs config.Attrs) error {
	var keys []string // collect and validate

	// Sort through to catch read-only keys
	values := make(configAttrs)
	for k, v := range attrs {
		if k == envconfig.AgentVersionKey {
			if c.ignoreReadOnlyFields {
				continue
			}
			return errors.Errorf(`%q must be set via "upgrade-model"`,
				envconfig.AgentVersionKey)
		} else if k == envconfig.CharmHubURLKey {
			if c.ignoreReadOnlyFields {
				continue
			}
			return errors.Errorf(`%q must be set via "add-model"`,
				envconfig.CharmHubURLKey)
		}

		values[k] = v
		keys = append(keys, k)
	}

	coerced, err := values.CoerceFormat()
	if err != nil {
		return errors.Trace(err)
	}

	if err := c.verifyKnownKeys(client, keys); err != nil {
		return errors.Trace(err)
	}

	return block.ProcessBlockedError(client.ModelSet(coerced), block.BlockChange)
}

// getConfig writes the value of a single model config key to the cmd.Context.
func (c *configCommand) getConfig(client configCommandAPI, ctx *cmd.Context) error {
	attrs, err := c.getFilteredModel(client)
	if err != nil {
		return err
	}

	if len(c.configBase.KeysToGet) == 0 {
		return errors.New("c.configBase.KeysToGet is empty")
	}

	key := c.configBase.KeysToGet[0]
	if value, found := attrs[key]; found {
		if c.out.Name() == "tabular" {
			// The user has not specified that they want
			// YAML or JSON formatting, so we print out
			// the value unadorned.
			return c.out.WriteFormatter(ctx, cmd.FormatSmart, value.Value)
		}
		// Return value in JSON / YAML format
		return c.out.Write(ctx, envconfig.ConfigValues{
			c.configBase.KeysToGet[0]: envconfig.ConfigValue{Source: value.Source, Value: value.Value},
		})
	}

	// Key not found - error
	mod, _ := c.ModelIdentifier()
	return errors.Errorf("%q is not a key of the currently targeted model: %q",
		key, mod)
}

// getAllConfig writes the full model config to the cmd.Context.
func (c *configCommand) getAllConfig(client configCommandAPI, ctx *cmd.Context) error {
	attrs, err := c.getFilteredModel(client)
	if err != nil {
		return err
	}

	// In tabular format, don't print "cloudinit-userdata" it can be very long,
	// instead give instructions on how to print specifically.
	if c.out.Name() == "tabular" {
		if value, ok := attrs[envconfig.CloudInitUserDataKey]; ok {
			if value.Value.(string) != "" {
				value.Value = "<value set, see juju model-config cloudinit-userdata>"
				attrs["cloudinit-userdata"] = value
			}
		}
	}

	return c.out.Write(ctx, attrs)
}

// getFilteredModel returns the model config with model attributes filtered out.
func (c *configCommand) getFilteredModel(client configCommandAPI) (envconfig.ConfigValues, error) {
	attrs, err := client.ModelGetWithMetadata()
	if err != nil {
		return nil, err
	}
	for attrName := range attrs {
		// We don't want model attributes included, these are available via show-model.
		if isModelAttribute(attrName) {
			delete(attrs, attrName)
		}
	}
	return attrs, nil
}

// verifyKnownKeys is a helper to validate the keys we are operating with
// against the set of known attributes from the model.
func (c *configCommand) verifyKnownKeys(client configCommandAPI, keys []string) error {
	known, err := client.ModelGet()
	if err != nil {
		return errors.Trace(err)
	}
	// AuthorizedKeys is a valid key, though you aren't allowed to set it.
	// This will be denied with a server error, but we don't want to warn
	// about an unknown key.
	known[envconfig.AuthorizedKeysKey] = ""

	allKeys := keys[:]
	for _, key := range allKeys {
		// Check if the key exists in the known config
		// and warn the user if the key is not defined.
		if _, exists := known[key]; !exists {
			logger.Warningf(
				"key %q is not defined in the current model configuration: possible misspelling", key)
		}
	}
	return nil
}

// isModelAttribute returns if the supplied attribute is a valid model
// attribute.
func isModelAttribute(attr string) bool {
	switch attr {
	case envconfig.NameKey, envconfig.TypeKey, envconfig.UUIDKey:
		return true
	}
	return false
}

// formatConfigTabular writes a tabular summary of config information.
func formatConfigTabular(writer io.Writer, value interface{}) error {
	configValues, ok := value.(envconfig.ConfigValues)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", configValues, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{
		TabWriter: tw,
	}

	var valueNames []string
	for name := range configValues {
		valueNames = append(valueNames, name)
	}
	sort.Strings(valueNames)
	w.Println("Attribute", "From", "Value")

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

// ConfigDetails returns the set of available configuration keys that can be set
// for a model along with printing information for the key containing a user
// readable description and the expected type for the key.
func ConfigDetails() (map[string]common.PrintConfigSchema, error) {
	defaultSchema, err := envconfig.Schema(nil)
	if err != nil {
		return nil, err
	}
	specifics := make(map[string]common.PrintConfigSchema, len(defaultSchema))
	for key, attr := range defaultSchema {
		if attr.Secret || isModelAttribute(key) ||
			attr.Group != environschema.EnvironGroup {
			continue
		}
		specifics[key] = common.PrintConfigSchema{
			Description: attr.Description,
			Type:        string(attr.Type),
		}
	}
	return specifics, nil
}
