// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package application

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/keyvalues"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

const maxValueSize = 5242880 // Max size for a config file.

const (
	configSummary = `Gets, sets, or resets configuration for a deployed application.`
	configDetails = `By default, all configuration (keys, values, metadata) for the application are
displayed if a key is not specified.

Output includes the name of the charm used to deploy the application and a
listing of the application-specific configuration settings.
See ` + "`juju status`" + ` for application names.

Examples:
    juju config apache2
    juju config --format=json apache2
    juju config mysql dataset-size
    juju config mysql --reset dataset-size,backup_dir
    juju config apache2 --file path/to/config.yaml
    juju config mysql dataset-size=80% backup_dir=/vol1/mysql/backups
    juju config apache2 --model mymodel --file /home/ubuntu/mysql.yaml

See also:
    deploy
    status
`
)

// NewConfigCommand returns a command used to get, reset, and set application
// attributes.
func NewConfigCommand() cmd.Command {
	return modelcmd.Wrap(&configCommand{})
}

type attributes map[string]string

// configCommand get, sets, and resets configuration values of an application.
type configCommand struct {
	api configCommandAPI
	modelcmd.ModelCommandBase
	out cmd.Output

	action          func(configCommandAPI, *cmd.Context) error // get, set, or reset action set in  Init
	applicationName string
	configFile      cmd.FileVar
	keys            []string
	reset           []string // Holds the keys to be reset until parsed.
	resetKeys       []string // Holds the keys to be reset once parsed.
	useFile         bool
	values          attributes
}

// configCommandAPI is an interface to allow passing in a fake implementation under test.
type configCommandAPI interface {
	Close() error
	Update(args params.ApplicationUpdate) error
	Get(application string) (*params.ApplicationGetResults, error)
	Set(application string, options map[string]string) error
	Unset(application string, options []string) error
}

// Info is part of the cmd.Command interface.
func (c *configCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "config",
		Args:    "<application name> [--reset <key[,key]>] [<attribute-key>][=<value>] ...]",
		Purpose: configSummary,
		Doc:     configDetails,
	}
}

// SetFlags is part of the cmd.Command interface.
func (c *configCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	f.Var(&c.configFile, "file", "path to yaml-formatted application config")
	f.Var(cmd.NewAppendStringsValue(&c.reset), "reset", "Reset the provided comma delimited keys")
}

// getAPI either uses the fake API set at test time or that is nil, gets a real
// API and sets that as the API.
func (c *configCommand) getAPI() (configCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := application.NewClient(root)
	return client, nil
}

// Init is part of the cmd.Command interface.
func (c *configCommand) Init(args []string) error {
	if len(args) == 0 || len(strings.Split(args[0], "=")) > 1 {
		return errors.New("no application name specified")
	}

	// If there are arguments provided to reset, we turn it into a slice of
	// strings and verify them. If there is one or more valid keys to reset and
	// no other errors initalizing the command, c.resetDefaults will be called
	// in c.Run.
	if err := c.parseResetKeys(); err != nil {
		return errors.Trace(err)
	}

	c.applicationName = args[0]
	args = args[1:]

	switch len(args) {
	case 0:
		return c.handleZeroArgs()
	case 1:
		return c.handleOneArg(args)
	default:
		return c.handleArgs(args)
	}
}

// handleZeroArgs handles the case where there are no positional args.
func (c *configCommand) handleZeroArgs() error {
	// If there's a path we're setting args from a file
	if c.configFile.Path != "" {
		return c.parseSet([]string{})
	}
	if len(c.reset) == 0 {
		// If there's nothing to reset we're getting all the settings.
		c.action = c.getConfig
	}
	// Otherwise just reset.
	return nil
}

// handleOneArg handles the case where there is one positional arg.
func (c *configCommand) handleOneArg(args []string) error {
	// If there's an '=', this must be setting a value
	if strings.Contains(args[0], "=") {
		return c.parseSet(args)
	}
	// If there's no reset,	we want to get a single value
	if len(c.reset) == 0 {
		c.action = c.getConfig
		c.keys = args
		return nil
	}
	// Otherwise we have reset and a get arg, which is invalid.
	return errors.New("cannot reset and retrieve values simultaneously")
}

// handleArgs handles the case where there's more than one positional arg.
func (c *configCommand) handleArgs(args []string) error {
	// This must be setting values but let's make sure.
	var pairs, numArgs int
	numArgs = len(args)
	for _, a := range args {
		if strings.Contains(a, "=") {
			pairs++
		}
	}
	if pairs == numArgs {
		return c.parseSet(args)
	}
	if pairs == 0 {
		return errors.New("can only retrieve a single value, or all values")
	}
	return errors.New("cannot set and retrieve values simultaneously")
}

// parseResetKeys splits the keys provided to --reset.
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
		if strings.Contains(k, "=") {
			return errors.Errorf(
				`--reset accepts a comma delimited set of keys "a,b,c", received: %q`, k)
		}
	}

	c.resetKeys = resetKeys
	return nil
}

// parseSet parses the command line args when --file is set or if the
// positional args are key=value pairs.
func (c *configCommand) parseSet(args []string) error {
	file := c.configFile.Path != ""
	if file && len(args) > 0 {
		return errors.New("cannot specify --file and key=value arguments simultaneously")
	}
	c.action = c.setConfig
	if file {
		c.useFile = true
		return nil
	}

	settings, err := keyvalues.Parse(args, true)
	if err != nil {
		return err
	}
	c.values = settings

	return nil
}

// Run implements the cmd.Command interface.
func (c *configCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	if len(c.resetKeys) > 0 {
		if err := c.resetConfig(client, ctx); err != nil {
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

// resetConfig is the run action when we are resetting attributes.
func (c *configCommand) resetConfig(client configCommandAPI, ctx *cmd.Context) error {
	return block.ProcessBlockedError(client.Unset(c.applicationName, c.resetKeys), block.BlockChange)
}

// setConfig is the run action when we are setting new attribute values as args
// or as a file passed in.
func (c *configCommand) setConfig(client configCommandAPI, ctx *cmd.Context) error {
	if c.useFile {
		return c.setConfigFromFile(client, ctx)
	}

	settings, err := c.validateValues(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	result, err := client.Get(c.applicationName)
	if err != nil {
		return err
	}

	for k, v := range settings {
		configValue := result.Config[k]

		configValueMap, ok := configValue.(map[string]interface{})
		if ok {
			// convert the value to string and compare
			if fmt.Sprintf("%v", configValueMap["value"]) == v {
				logger.Warningf("the configuration setting %q already has the value %q", k, v)
			}
		}
	}

	return block.ProcessBlockedError(client.Set(c.applicationName, settings), block.BlockChange)
}

// setConfigFromFile sets the application configuration from settings passed
// in a YAML file.
func (c *configCommand) setConfigFromFile(client configCommandAPI, ctx *cmd.Context) error {
	var (
		b   []byte
		err error
	)
	if c.configFile.Path == "-" {
		buf := bytes.Buffer{}
		buf.ReadFrom(ctx.Stdin)
		b = buf.Bytes()
	} else {
		b, err = c.configFile.Read(ctx)
		if err != nil {
			return err
		}
	}
	return block.ProcessBlockedError(
		client.Update(
			params.ApplicationUpdate{
				ApplicationName: c.applicationName,
				SettingsYAML:    string(b)}), block.BlockChange)
}

// getConfig is the run action to return one or all configuration values.
func (c *configCommand) getConfig(client configCommandAPI, ctx *cmd.Context) error {
	results, err := client.Get(c.applicationName)
	if err != nil {
		return err
	}
	if len(c.keys) == 1 {
		key := c.keys[0]
		info, found := results.Config[key].(map[string]interface{})
		if !found {
			return errors.Errorf("key %q not found in %q application settings.", key, c.applicationName)
		}
		out := &bytes.Buffer{}
		err := cmd.FormatYaml(out, info["value"])
		if err != nil {
			return err
		}
		fmt.Fprint(ctx.Stdout, out.String())
		return nil
	}

	resultsMap := map[string]interface{}{
		"application": results.Application,
		"charm":       results.Charm,
		"settings":    results.Config,
	}
	return c.out.Write(ctx, resultsMap)
}

// validateValues reads the values provided as args and validates that they are
// valid UTF-8.
func (c *configCommand) validateValues(ctx *cmd.Context) (map[string]string, error) {
	settings := map[string]string{}
	for k, v := range c.values {
		//empty string is also valid as a setting value
		if v == "" {
			settings[k] = v
			continue
		}

		if v[0] != '@' {
			if !utf8.ValidString(v) {
				return nil, errors.Errorf("value for option %q contains non-UTF-8 sequences", k)
			}
			settings[k] = v
			continue
		}
		nv, err := readValue(ctx, v[1:])
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !utf8.ValidString(nv) {
			return nil, errors.Errorf("value for option %q contains non-UTF-8 sequences", k)
		}
		settings[k] = nv
	}
	return settings, nil
}

// readValue reads the value of an option out of the named file.
// An empty content is valid, like in parsing the options. The upper
// size is 5M.
func readValue(ctx *cmd.Context, filename string) (string, error) {
	absFilename := ctx.AbsPath(filename)
	fi, err := os.Stat(absFilename)
	if err != nil {
		return "", errors.Errorf("cannot read option from file %q: %v", filename, err)
	}
	if fi.Size() > maxValueSize {
		return "", errors.Errorf("size of option file is larger than 5M")
	}
	content, err := ioutil.ReadFile(ctx.AbsPath(filename))
	if err != nil {
		return "", errors.Errorf("cannot read option from file %q: %v", filename, err)
	}
	return string(content), nil
}
