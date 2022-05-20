// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/yaml.v3"

	apicontroller "github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/config"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/controller"
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
When run with no arguments, this command displays the whole configuration
(keys and values) for the controller. Supplying a single key returns the value
for that key.

Supplying one or more key=value pairs will set the provided keys to those
values. You can also set config values from a yaml file using the --file flag.
Not all keys can be updated after bootstrap time.

By default, all commands target the currently selected controller. You
can target a different controller by using the -c flag.

`
	controllerConfigHelpDocKeys = `
The following keys are available:
`
	configCommandHelpDocPart2 = `

Examples:

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

See also:
    controllers
    model-config
    show-cloud
`
)

// Info returns information about this command - it's part of
// cmd.Command.
func (c *configCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "controller-config",
		Args:    "[<attribute key>[=<value>] ...]",
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
	ControllerConfig() (controller.Config, error)
	ConfigSet(map[string]interface{}) error
}

func (c *configCommand) getAPI() (controllerAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicontroller.NewClient(root), nil
}

// Run executes the command as directed by the options and
// arguments. It's part of cmd.Command.
func (c *configCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	switch c.configBase.Actions[0] {
	case config.GetOne:
		return c.getConfig(client, ctx)
	case config.Set:
		return c.setConfig(client, ctx)
	case config.SetFile:
		return c.setConfigFile(client, ctx)
	default:
		return c.getAllConfig(client, ctx)
	}
}

// getAllConfig returns the entire configuration for the selected controller.
func (c *configCommand) getAllConfig(client controllerAPI, ctx *cmd.Context) error {
	attrs, err := client.ControllerConfig()
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
	attrs, err := client.ControllerConfig()
	if err != nil {
		return err
	}
	if value, found := attrs[c.configBase.KeyToGet]; found {
		if c.out.Name() == "tabular" {
			// The user has not specified that they want
			// YAML or JSON formatting, so we print out
			// the value unadorned.
			return c.out.WriteFormatter(ctx, cmd.FormatSmart, value)
		}
		return c.out.Write(ctx, value)
	}
	return errors.Errorf("key %q not found in controller %q",
		c.configBase.KeyToGet, controllerName)
}

// setConfig sets config values from provided key=value arguments.
func (c *configCommand) setConfig(client controllerAPI, ctx *cmd.Context) error {
	attrs := make(map[string]interface{})
	for key, value := range c.configBase.ValsToSet {
		attrs[key] = value
	}
	values, err := c.filterAttrs(attrs)
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(client.ConfigSet(values))
}

// setConfigFile sets config values from the provided yaml file.
func (c *configCommand) setConfigFile(client controllerAPI, ctx *cmd.Context) error {
	// Read file & unmarshal into yaml
	path, err := utils.NormalizePath(c.configBase.ConfigFile.Path)
	if err != nil {
		return errors.Trace(err)
	}
	data, err := os.ReadFile(ctx.AbsPath(path))
	if err != nil {
		return errors.Trace(err)
	}
	attrs := make(map[string]interface{})
	if err := yaml.Unmarshal(data, &attrs); err != nil {
		return errors.Trace(err)
	}

	values, err := c.filterAttrs(attrs)
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(client.ConfigSet(values))
}

// filterAttrs checks if any of the `attrs` being set are unsettable. If so,
// it will filter them out, and either log a warning or return an error
// (depending on the value of `c.ignoreReadOnlyFields`).
func (c *configCommand) filterAttrs(attrs map[string]interface{}) (map[string]interface{}, error) {
	store := c.ClientStore()
	controllerName, err := store.CurrentController()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctrl, err := store.ControllerByName(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, err = controller.NewConfig(ctrl.ControllerUUID, ctrl.CACert, attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	extraValues := set.NewStrings()
	values := make(map[string]interface{})
	for k := range attrs {
		if controller.AllowedUpdateConfigAttributes.Contains(k) {
			values[k] = attrs[k]
		} else {
			extraValues.Add(k)
		}
	}
	if extraValues.Size() > 0 {
		if c.ignoreReadOnlyFields {
			logger.Warningf("invalid or read-only controller config values ignored: %v", extraValues.SortedValues())
		} else {
			return nil, errors.Errorf("invalid or read-only controller config values cannot be updated: %v", extraValues.SortedValues())
		}
	}
	return values, nil
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
func ConfigDetailsAll() (map[string]interface{}, error) {
	specifics := make(map[string]interface{})
	for key, attr := range controller.ConfigSchema {
		specifics[key] = attrToPrintSchema(attr)
	}
	return specifics, nil
}

func attrToPrintSchema(attr environschema.Attr) common.PrintConfigSchema {
	return common.PrintConfigSchema{
		Description: attr.Description,
		Type:        fmt.Sprintf("%s", attr.Type),
	}
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
