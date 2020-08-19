// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apicontroller "github.com/juju/juju/api/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/controller"
)

// NewConfigCommand returns a new command that can retrieve or update
// controller configuration.
func NewConfigCommand() cmd.Command {
	return modelcmd.WrapController(&configCommand{})
}

// configCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type configCommand struct {
	modelcmd.ControllerCommandBase
	api controllerAPI
	out cmd.Output

	action     func(controllerAPI, *cmd.Context) error // The action we want to perform, set in cmd.Init.
	key        string                                  // One config key to read.
	setOptions common.ConfigFlag                       // Config values to set.
}

const (
	configCommandHelpDocPart1 = `
By default, all configuration (keys and values) for the controller are
displayed if a key is not specified. Supplying one key name returns
only the value for that key.

Supplying key=value will set the supplied key to the supplied value;
this can be repeated for multiple keys. You can also specify a yaml
file containing key values. Not all keys can be updated after
bootstrap time.

`
	controllerConfigHelpDocKeys = `
The following keys are available:
`
	configCommandHelpDocPart2 = `

Examples:

    juju controller-config
    juju controller-config api-port
    juju controller-config -c mycontroller
    juju controller-config auditing-enabled=true audit-log-max-backups=5
    juju controller-config auditing-enabled=true path/to/file.yaml
    juju controller-config path/to/file.yaml

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
	if details, err := ConfigDetails(); err == nil {
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
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"json":    cmd.FormatJson,
		"tabular": formatConfigTabular,
		"yaml":    cmd.FormatYaml,
	})
}

// Init initialised the command from the arguments - it's part of
// cmd.Command.
func (c *configCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return c.handleZeroArgs()
	case 1:
		return c.handleOneArg(args[0])
	default:
		return c.handleArgs(args)
	}
}

func (c *configCommand) handleZeroArgs() error {
	c.action = c.getConfig
	return nil
}

func (c *configCommand) handleOneArg(arg string) error {
	// We may have a single config.yaml file
	_, err := c.Filesystem().Stat(arg)
	if err == nil || strings.Contains(arg, "=") {
		return c.parseSetKeys([]string{arg})
	}
	c.key = arg
	c.action = c.getConfig
	return nil
}

func (c *configCommand) handleArgs(args []string) error {
	if err := c.parseSetKeys(args); err != nil {
		return errors.Trace(err)
	}
	for _, arg := range args {
		// We may have a config.yaml file.
		_, err := c.Filesystem().Stat(arg)
		if err != nil && !strings.Contains(arg, "=") {
			return errors.New("can only retrieve a single value, or all values")
		}
	}
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
	return c.action(client, ctx)
}

func (c *configCommand) getConfig(client controllerAPI, ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	attrs, err := client.ControllerConfig()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			if c.out.Name() == "tabular" {
				// The user has not specified that they want
				// YAML or JSON formatting, so we print out
				// the value unadorned.
				return c.out.WriteFormatter(ctx, cmd.FormatSmart, value)
			}
			return c.out.Write(ctx, value)
		}
		return errors.Errorf("key %q not found in %q controller", c.key, controllerName)
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

func (c *configCommand) setConfig(client controllerAPI, ctx *cmd.Context) error {
	attrs, err := c.setOptions.ReadAttrs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(client.ConfigSet(attrs))
}

// ConfigDetails gets information about controller config attributes.
func ConfigDetails() (map[string]interface{}, error) {
	specifics := make(map[string]interface{})
	for key, attr := range controller.ConfigSchema {
		if !controller.AllowedUpdateConfigAttributes.Contains(key) {
			continue
		}
		specifics[key] = common.PrintConfigSchema{
			Description: attr.Description,
			Type:        fmt.Sprintf("%s", attr.Type),
		}
	}
	return specifics, nil
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
