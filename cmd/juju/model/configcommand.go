// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/api/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/environs/config"
)

const (
	modelConfigSummary        = "Displays or sets configuration values on a model."
	modelConfigHelpDocPartOne = `
By default, all configuration (keys, source, and values) for the current model
are displayed.

Supplying one key name returns only the value for the key. Supplying key=value
will set the supplied key to the supplied value, this can be repeated for
multiple keys. You can also specify a yaml file containing key values, that can
be used for the input for the command.
`
	modelConfigHelpDocKeys = `
The following keys are available:
`
	modelConfigHelpDocPartTwo = `
Examples:
    juju model-config default-series
    juju model-config -m mycontroller:mymodel
    juju model-config ftp-proxy=10.0.0.1:8000
    juju model-config ftp-proxy=10.0.0.1:8000 path/to/file.yaml
    juju model-config path/to/file.yaml
    juju model-config -m othercontroller:mymodel default-series=yakkety test-mode=false
    juju model-config --reset default-series test-mode

See also:
    models
    model-defaults
    show-cloud
    controller-config
`
)

// NewConfigCommand wraps configCommand with sane model settings.
func NewConfigCommand() cmd.Command {
	return modelcmd.Wrap(&configCommand{})
}

type attributes map[string]interface{}

// CoerceFormat attempts to convert the attributes values from the complex type
// to the more simple type. This is because the output of this command outputs
// in the following format:
//
//     resource-name:
//        value: foo
//        source: default
//
// Where the consuming side of the command expects it in the following format:
//
//     resource-name: foo
//
// CoerceFormat attempts to diagnose this and attempt to do this correctly.
func (a attributes) CoerceFormat() (attributes, error) {
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
	api configCommandAPI
	modelcmd.ModelCommandBase
	out cmd.Output

	action              func(configCommandAPI, *cmd.Context) error // The action which we want to handle, set in cmd.Init.
	keys                []string
	reset               []string // Holds the keys to be reset until parsed.
	resetKeys           []string // Holds the keys to be reset once parsed.
	setOptions          common.ConfigFlag
	ignoreAgentVersion  bool
	skipImmutableErrors bool
}

// configCommandAPI defines an API interface to be used during testing.
type configCommandAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
	ModelGetWithMetadata() (config.ConfigValues, error)
	ModelSet(config map[string]interface{}) error
	ModelUnset(keys ...string) error
}

// Info implements part of the cmd.Command interface.
func (c *configCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Args:    "[<model-key>[=<value>] ...]",
		Name:    "model-config",
		Purpose: modelConfigSummary,
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

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"tabular": formatConfigTabular,
		"yaml":    cmd.FormatYaml,
	})
	f.Var(cmd.NewAppendStringsValue(&c.reset), "reset", "Reset the provided comma delimited keys")
	f.BoolVar(&c.ignoreAgentVersion, "ignore-agent-version", false, "Skip the error when passing in the agent version configuration (deprecated)")
	f.BoolVar(&c.skipImmutableErrors, "skip-immutable-errors", false, "Skip immutable errors when passing in the configurations")
}

// Init implements part of the cmd.Command interface.
func (c *configCommand) Init(args []string) error {
	// If there are arguments provided to reset, we turn it into a slice of
	// strings and verify them. If there is one or more valid keys to reset and
	// no other errors initializing the command, c.resetDefaults will be called
	// in c.Run.
	if err := c.parseResetKeys(); err != nil {
		return errors.Trace(err)
	}

	switch len(args) {
	case 0:
		return c.handleZeroArgs()
	case 1:
		return c.handleOneArg(args[0])
	default:
		return c.handleArgs(args)
	}
}

// handleZeroArgs handles the case where there are no positional args.
func (c *configCommand) handleZeroArgs() error {
	// If reset is empty we're getting configuration
	if len(c.reset) == 0 {
		c.action = c.getConfig
	}
	// Otherwise we're going to reset args.
	return nil
}

// handleOneArg handles the case where there is one positional arg.
func (c *configCommand) handleOneArg(arg string) error {
	if arg == "-" {
		// If we can't read the stdin, then continue onwards to fall back to the
		// previous logic.
		if err := c.parseStdin(); err == nil {
			return nil
		}
	}

	// We may have a single config.yaml file
	if _, err := os.Stat(arg); err == nil {
		return c.parseYAMLFile(arg)
	} else if strings.Contains(arg, "=") {
		return c.parseSetKeys([]string{arg})
	}
	// If we are not setting a value, then we are retrieving one so we need to
	// make sure that we are not resetting because it is not valid to get and
	// reset simultaneously.
	if len(c.reset) > 0 {
		return errors.New("cannot set and retrieve model values simultaneously")
	}
	c.keys = []string{arg}
	c.action = c.getConfig
	return ParseCert(arg)
}

// handleArgs handles the case where there's more than one positional arg.
func (c *configCommand) handleArgs(args []string) error {
	if err := c.parseSetKeys(args); err != nil {
		return errors.Trace(err)
	}
	for _, arg := range args {
		// We may have a config.yaml file.
		_, err := os.Stat(arg)
		if err != nil && !strings.Contains(arg, "=") {
			return errors.New("can only retrieve a single value, or all values")
		}
	}
	return nil
}

// parseStdin ensures that we handle stdin correctly.
func (c *configCommand) parseStdin() error {
	if err := c.setOptions.SetAttrsFromReader(os.Stdin); err != nil {
		return errors.Trace(err)
	}
	c.action = c.setConfig
	return nil
}

// parseYAMLFile ensures that we handle the YAML file passed in.
func (c *configCommand) parseYAMLFile(file string) error {
	if err := c.setOptions.Set(file); err != nil {
		return errors.Trace(err)
	}
	c.action = c.setConfig
	return nil
}

// parseSetKeys iterates over the args and make sure that the key=value pairs
// are valid.
func (c *configCommand) parseSetKeys(args []string) error {
	for _, arg := range args {
		if err := c.setOptions.Set(arg); err != nil {
			return errors.Trace(err)
		}
	}
	c.action = c.setConfig
	return nil
}

// parseResetKeys splits the keys provided to --reset after trimming any
// leading or trailing comma. It then verifies that we haven't incorrectly
// received any key=value pairs and finally sets the value(s) on c.resetKeys.
func (c *configCommand) parseResetKeys() error {
	if len(c.reset) == 0 {
		return nil
	}
	var resetKeys []string
	for _, value := range c.reset {
		keys := strings.Split(strings.Trim(value, ","), ",")
		resetKeys = append(resetKeys, keys...)
	}

	for _, k := range resetKeys {
		if k == config.AgentVersionKey || k == config.CharmhubURLKey {
			return errors.Errorf("%q cannot be reset", k)
		}
		if strings.Contains(k, "=") {
			return errors.Errorf(
				`--reset accepts a comma delimited set of keys "a,b,c", received: %q`, k)
		}
	}
	c.resetKeys = resetKeys
	return nil
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

	if len(c.resetKeys) > 0 {
		err := c.resetConfig(client, ctx)
		if err != nil {
			// We return this error naked as it is almost certainly going to be
			// cmd.ErrSilent and the cmd.Command framework expects that back
			// from cmd.Run if the process is blocked.
			return err
		}
	}
	if c.action == nil {
		// If we are reset only we end up here, only we've already done that.
		return nil
	}
	return c.action(client, ctx)
}

// resetConfig unsets the keys provided to the command.
func (c *configCommand) resetConfig(client configCommandAPI, ctx *cmd.Context) error {
	// ctx unused in this method
	if err := c.verifyKnownKeys(client, c.resetKeys); err != nil {
		return errors.Trace(err)
	}

	return block.ProcessBlockedError(client.ModelUnset(c.resetKeys...), block.BlockChange)
}

// setConfig sets the provided key/value pairs on the model.
func (c *configCommand) setConfig(client configCommandAPI, ctx *cmd.Context) error {
	attrs, err := c.setOptions.ReadAttrs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	var keys []string
	values := make(attributes)
	for k, v := range attrs {
		if k == config.AgentVersionKey {
			if c.ignoreAgentVersion || c.skipImmutableErrors {
				continue
			}
			return errors.Errorf(`"agent-version" must be set via "upgrade-model"`)
		} else if k == config.CharmhubURLKey {
			if c.skipImmutableErrors {
				continue
			}
			return errors.Errorf(`"charmhub-url" must be set via "add-model"`)
		}

		values[k] = v
		keys = append(keys, k)
	}

	coerced, err := values.CoerceFormat()
	if err != nil {
		return errors.Trace(err)
	}

	for _, k := range c.resetKeys {
		if _, ok := coerced[k]; ok {
			return errors.Errorf(
				"key %q cannot be both set and reset in the same command", k)
		}
	}

	if err := c.verifyKnownKeys(client, keys); err != nil {
		return errors.Trace(err)
	}

	return block.ProcessBlockedError(client.ModelSet(coerced), block.BlockChange)
}

// get writes the value of a single key or the full output for the model to the cmd.Context.
func (c *configCommand) getConfig(client configCommandAPI, ctx *cmd.Context) error {
	if len(c.keys) == 1 && certBytes != nil {
		_, _ = ctx.Stdout.Write(certBytes)
		return nil
	}
	attrs, err := c.getFilteredModel(client)
	if err != nil {
		return err
	}
	attrs, err, finished := c.handleIsKeyOfModel(attrs, ctx)
	if err != nil {
		return err
	} else if attrs != nil && finished {
		return c.out.Write(ctx, attrs)
	} else if len(c.keys) > 0 && !finished {
		if isFileLike(c.keys[0]) {
			return errors.Errorf("%q seems to be a file but not found", c.keys[0])
		} else {
			mod, _ := c.ModelIdentifier()
			return errors.Errorf("%q seems to be neither a file nor a key of the currently targeted model: %q",
				c.keys[0], mod)
		}
	}
	return nil
}

func (c *configCommand) getFilteredModel(client configCommandAPI) (config.ConfigValues, error) {
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

func isFileLike(fileLike string) bool {
	r, _ := regexp.Compile("\\.[a-zA-Z]{0,4}$")
	match := r.FindString(fileLike)
	if strings.HasSuffix(match, ".") {
		return false
	}
	if match == "" || match == " " {
		return false
	}
	return true
}

func (c *configCommand) handleIsKeyOfModel(attrs config.ConfigValues, ctx *cmd.Context) (config.ConfigValues, error, bool) {
	if len(c.keys) == 1 {
		key := c.keys[0]
		if value, found := attrs[key]; found {
			if c.out.Name() == "tabular" {
				// The user has not specified that they want
				// YAML or JSON formatting, so we print out
				// the value unadorned.
				return nil, c.out.WriteFormatter(ctx, cmd.FormatSmart, value.Value), true
			} else {
				return config.ConfigValues{key: config.ConfigValue{Source: value.Source, Value: value.Value}}, nil, true
			}
		} else {
			return attrs, nil, false
		}
	} else {
		// In tabular format, don't print "cloudinit-userdata" it can be very long,
		// instead give instructions on how to print specifically.
		if value, ok := attrs[config.CloudInitUserDataKey]; ok && c.out.Name() == "tabular" {
			if value.Value.(string) != "" {
				value.Value = "<value set, see juju model-config cloudinit-userdata>"
				attrs["cloudinit-userdata"] = value
			}
			return attrs, nil, true
		}
	}
	return attrs, nil, true
}

// verifyKnownKeys is a helper to validate the keys we are operating with
// against the set of known attributes from the model.
func (c *configCommand) verifyKnownKeys(client configCommandAPI, keys []string) error {
	known, err := client.ModelGet()
	if err != nil {
		return errors.Trace(err)
	}

	allKeys := keys[:]
	for _, key := range allKeys {
		// check if the key exists in the known config
		// and warn the user if the key is not defined
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
	case config.NameKey, config.TypeKey, config.UUIDKey:
		return true
	}
	return false
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

// ConfigDetails gets ModelDetails when a model is not available
// to use.
func ConfigDetails() (map[string]interface{}, error) {
	defaultSchema, err := config.Schema(nil)
	if err != nil {
		return nil, err
	}
	specifics := make(map[string]interface{})
	for key, attr := range defaultSchema {
		if attr.Secret || isModelAttribute(key) ||
			attr.Group != environschema.EnvironGroup {
			continue
		}
		specifics[key] = common.PrintConfigSchema{
			Description: attr.Description,
			Type:        fmt.Sprintf("%s", attr.Type),
		}
	}
	return specifics, nil
}
