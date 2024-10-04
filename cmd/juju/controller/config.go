// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/environschema.v1"

	apicontroller "github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/config"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/output"
)

var ctrConfigBase = config.ConfigCommandBase{
	Resettable: false,
}

// NewConfigCommand returns a new command that can retrieve or update
// controller configuration.
func NewConfigCommand() cmd.Command {
	return modelcmd.WrapController(&configCommand{configBase: ctrConfigBase})
}

// configCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type configCommand struct {
	modelcmd.ControllerCommandBase
	configBase config.ConfigCommandBase
	api        controllerAPI
	out        cmd.Output

	// Extra `controller-config`-specific fields
	ignoreReadOnlyFields bool
}

const (
	configCommandHelpDocPart1 = `
To view all configuration values for the current controller, run
    juju controller-config
You can target a specific controller using the -c flag:
    juju controller-config -c <controller>
By default, the config will be printed in a tabular format. You can instead
print it in json or yaml format using the --format flag:
    juju controller-config --format json
    juju controller-config --format yaml

To view the value of a single config key, run
    juju controller-config key
To set config values, run
    juju controller-config key1=val1 key2=val2 ...

Config values can be imported from a yaml file using the --file flag:
    juju controller-config --file=path/to/cfg.yaml
This allows you to e.g. save a controller's config to a file:
    juju controller-config --format=yaml > cfg.yaml
and then import the config later. Note that the output of controller-config
may include read-only values, which will cause an error when importing later.
To prevent the error, use the --ignore-read-only-fields flag:
    juju controller-config --file=cfg.yaml --ignore-read-only-fields

You can also read from stdin using "-", which allows you to pipe config values
from one controller to another:
    juju controller-config -c c1 --format=yaml \
      | juju controller-config -c c2 --file=- --ignore-read-only-fields
You can simultaneously read config from a yaml file and set config keys
as above. The command-line args will override any values specified in the file.
`
	controllerConfigHelpDocKeys = `
The following keys are available:
`
	configCommandHelpDocPart2 = `
`
	configCommandHelpExamples = `
Print all config values for the current controller:

    juju controller-config

Print the value of "api-port" for the current controller:

    juju controller-config api-port

Print all config values for the controller "mycontroller":

    juju controller-config -c mycontroller

Set the "auditing-enabled" and "audit-log-max-backups" keys:

    juju controller-config auditing-enabled=true audit-log-max-backups=5

Set the current controller's config from a yaml file:

    juju controller-config --file path/to/file.yaml
`
)

// Info returns information about this command - it's part of
// cmd.Command.
func (c *configCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:     "controller-config",
		Args:     "[<attribute key>[=<value>] ...]",
		Examples: configCommandHelpExamples,
		SeeAlso: []string{
			"controllers",
			"model-config",
			"show-cloud",
		},
		Purpose: "Displays or sets configuration settings for a controller.",
	}
	if details, err := ConfigDetailsUpdatable(); err == nil {
		if formattedDetails, err := common.FormatConfigSchema(details); err == nil {
			info.Doc = fmt.Sprintf("%s%s\n%s%s",
				configCommandHelpDocPart1,
				controllerConfigHelpDocKeys,
				formattedDetails,
				configCommandHelpDocPart2)
			return jujucmd.Info(info)
		}
	}
	info.Doc = strings.TrimSpace(fmt.Sprintf("%s%s",
		configCommandHelpDocPart1,
		configCommandHelpDocPart2))

	return jujucmd.Info(info)
}

// SetFlags adds command-specific flags to the flag set. It's part of
// cmd.Command.
func (c *configCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.configBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"tabular": formatConfigTabular,
		"yaml":    cmd.FormatYaml,
	})
	f.BoolVar(&c.ignoreReadOnlyFields, "ignore-read-only-fields", false, "Ignore read only fields that might cause errors to be emitted while processing yaml documents")
}

// Init initialised the command from the arguments - it's part of
// cmd.Command.
func (c *configCommand) Init(args []string) error {
	return c.configBase.Init(args)
}

type controllerAPI interface {
	Close() error
	ControllerConfig(context.Context) (controller.Config, error)
	ConfigSet(context.Context, map[string]interface{}) error
}

func (c *configCommand) getAPI(ctx context.Context) (controllerAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicontroller.NewClient(root), nil
}

// Run executes the command as directed by the options and
// arguments. It's part of cmd.Command.
func (c *configCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI(ctx)
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
			err = c.setConfig(ctx, client, c.configBase.ValsToSet)
		case config.SetFile:
			var attrs config.Attrs
			attrs, err = c.configBase.ReadFile(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			err = c.setConfig(ctx, client, attrs)
		default:
			err = c.getAllConfig(client, ctx)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// getAllConfig returns the entire configuration for the selected controller.
func (c *configCommand) getAllConfig(client controllerAPI, ctx *cmd.Context) error {
	attrs, err := client.ControllerConfig(ctx.Context)
	if err != nil {
		return err
	}
	// Return whole config
	return c.out.Write(ctx, attrs)
}

// getConfig returns the value of the specified key.
func (c *configCommand) getConfig(client controllerAPI, ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	attrs, err := client.ControllerConfig(ctx.Context)
	if err != nil {
		return err
	}
	if len(c.configBase.KeysToGet) == 0 {
		return errors.New("c.configBase.KeysToGet is empty")
	}
	if value, found := attrs[c.configBase.KeysToGet[0]]; found {
		if c.out.Name() == "tabular" {
			// The user has not specified that they want
			// YAML or JSON formatting, so we print out
			// the value unadorned.
			return c.out.WriteFormatter(ctx, cmd.FormatSmart, value)
		}
		return c.out.Write(ctx, value)
	}
	return errors.Errorf("key %q not found in controller %q",
		c.configBase.KeysToGet[0], controllerName)
}

// filterOutReadOnly removes in-situ read-only attributes from the provided configuration attributes map.
func (c *configCommand) filterOutReadOnly(attrs config.Attrs) error {
	extraValues := set.NewStrings()
	for k := range attrs {
		if !controller.AllowedUpdateConfigAttributes.Contains(k) {
			extraValues.Add(k)
			delete(attrs, k)
		}
	}

	// No readonly
	if extraValues.Size() == 0 {
		return nil
	}
	if !c.ignoreReadOnlyFields {
		return errors.Errorf("invalid or read-only controller config values cannot be updated: %v", extraValues.SortedValues())
	}

	logger.Warningf("invalid or read-only controller config values ignored: %v", extraValues.SortedValues())
	return nil
}

// setConfig sets config values from the provided config.Attrs.
func (c *configCommand) setConfig(ctx context.Context, client controllerAPI, attrs config.Attrs) error {
	err := c.filterOutReadOnly(attrs)
	if err != nil {
		return errors.Trace(err)
	}

	store := c.ClientStore()
	controllerName, err := store.CurrentController()
	if err != nil {
		return errors.Trace(err)
	}
	ctrl, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	// Despite its name, NewConfig has a desired side effect:
	// It replaces all complex string values from `attrs` by their object counter part.
	// ex: [value1,value2] will be replaced by a slice []string{"value1", "value2"}
	_, err = controller.NewConfig(ctrl.ControllerUUID, ctrl.CACert, attrs)
	if err != nil {
		return errors.Trace(err)
	}

	// Check if any of the `attrs` are not allowed to be set
	fields, _, err := controller.ConfigSchema.ValidationSchema()
	if err != nil {
		return errors.Trace(err)
	}

	values := make(map[string]interface{})
	for k := range attrs {
		if field, ok := fields[k]; ok {
			v, err := field.Coerce(attrs[k], []string{k})
			if err != nil {
				return err
			}
			values[k] = v
		} else {
			values[k] = attrs[k]
		}
	}

	return errors.Trace(client.ConfigSet(ctx, values))
}

// ConfigDetailsUpdatable gets information about the controller config
// attributes that are updatable.
func ConfigDetailsUpdatable() (map[string]interface{}, error) {
	specifics := make(map[string]interface{})
	for key, attr := range controller.ConfigSchema {
		if !controller.AllowedUpdateConfigAttributes.Contains(key) {
			continue
		}
		specifics[key] = attrToPrintSchema(attr)
	}
	return specifics, nil
}

// ConfigDetailsAll gets information about all the controller config
// attributes, including those only settable during bootstrap.
func ConfigDetailsAll() (map[string]common.PrintConfigSchema, error) {
	specifics := make(map[string]common.PrintConfigSchema, len(controller.ConfigSchema))
	for key, attr := range controller.ConfigSchema {
		specifics[key] = attrToPrintSchema(attr)
	}
	return specifics, nil
}

func attrToPrintSchema(attr environschema.Attr) common.PrintConfigSchema {
	return common.PrintConfigSchema{
		Description: attr.Description,
		Type:        string(attr.Type),
	}
}

func formatConfigTabular(writer io.Writer, value interface{}) error {
	controllerConfig, ok := value.(controller.Config)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", controllerConfig, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{TabWriter: tw}

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

		// Special formatting for multiline exclude-methods lists.
		if name == controller.AuditLogExcludeMethods {
			if strings.Contains(valString, "\n") {
				valString = "\n" + valString
			} else {
				valString = strings.TrimLeft(valString, "- ")
			}
		}

		w.Println(name, valString)
	}

	w.Flush()
	return nil
}
