// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/config"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc/params"
)

const (
	configSummary = `Get, set, or reset configuration for a deployed application.`
	configDetails = `
To view all configuration values for an application:

    juju config <app>

By default, the config will be printed in a ` + "`yaml`" + ` format. You can instead print it
in a ` + "`json`" + ` format using the ` + "`--format`" + ` flag:

    juju config <app> --format json

To view the value of a single config key, run

    juju config <app> key

To set config values, run

    juju config <app> key1=val1 key2=val2 ...

This sets "key1" to "val1", etc. Using the ` + "`@`" + ` directive, you can set a config
key's value to the contents of a file:

    juju config <app> key=@/tmp/configvalue

You can also reset config keys to their default values:

    juju config <app> --reset key1
    juju config <app> --reset key1,key2,key3

You may simultaneously set some keys and reset others:

    juju config <app> key1=val1 key2=val2 --reset key3,key4

Config values can be imported from a yaml file using the --file flag:

    juju config <app> --file=path/to/cfg.yaml

The ` + "`yaml`" + ` file should be in the following format:

    apache2:                        # application name
      servername: "example.com"     # key1: val1
      lb_balancer_timeout: 60       # key2: val2
      ...

This allows you to, e.g., save an app's config to a file:

    juju config app1 > cfg.yaml

and then import the config later. You can also read from stdin using ` + "`-`" + `,
which allows you to pipe config values from one app to another:

    juju config app1 | juju config app2 --file -

You can simultaneously read config from a yaml file and set/reset config keys
as above. The command-line args will override any values specified in the file.

Rather than specifying each setting name/value inline, the ` + "`--file`" + ` flag option
may be used to provide a list of settings to be updated as a yaml file. The
yaml file contents must include a single top-level key with the application's
name followed by a dictionary of key/value pairs that correspond to the names
and values of the settings to be set. For instance, to configure apache2,
the following yaml file can be used:

    apache2:
      servername: "example.com"
      lb_balancer_timeout: 60

If the above ` + "`yaml`" + ` document is stored in a file called ` + "`config.yaml`" + `, the
following command can be used to apply the config changes:

    juju config apache2 --file config.yaml

Finally, the ` + "`--reset`" + ` flag can be used to revert one or more configuration
settings back to their default value as defined in the charm metadata:

    juju config apache2 --reset servername
    juju config apache2 --reset servername,lb_balancer_timeout
`

	examples = `
To view all configuration values for an application, run

    juju config mysql --format json

To set a configuration value for an application, run

    juju config mysql foo=bar

To set some keys and reset others:

    juju config mysql key1=val1 key2=val2 --reset key3,key4

To set a configuration value for an application from a file:

    juju config mysql --file=path/to/cfg.yaml
`
)

var appConfigBase = config.ConfigCommandBase{
	Resettable: true,
}

// NewConfigCommand returns a command used to get, reset, and set application
// charm attributes.
func NewConfigCommand() cmd.Command {
	return modelcmd.Wrap(&configCommand{configBase: appConfigBase})
}

// configCommand get, sets, and resets configuration values of an application' charm.
type configCommand struct {
	modelcmd.ModelCommandBase
	configBase config.ConfigCommandBase
	api        ApplicationAPI
	out        cmd.Output

	// Extra `juju config` specific fields
	applicationName string
	branchName      string
}

// ApplicationAPI is an interface to allow passing in a fake implementation under test.
type ApplicationAPI interface {
	Close() error
	Get(branchName string, application string) (*params.ApplicationGetResults, error)
	SetConfig(branchName string, application, configYAML string, config map[string]string) error
	UnsetApplicationConfig(branchName string, application string, options []string) error
}

// Info is part of the cmd.Command interface.
func (c *configCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "config",
		Args:     "<application name> [--reset <key[,key]>] [<attribute-key>][=<value>] ...]",
		Purpose:  configSummary,
		Doc:      configDetails,
		Examples: examples,
		SeeAlso: []string{
			"deploy",
			"status",
			"model-config",
			"controller-config",
		},
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *configCommand) SetFlags(f *gnuflag.FlagSet) {
	// Set the -B / --no-browser-login flag, and model/controller specific flags
	c.ModelCommandBase.SetFlags(f)
	// Set ConfigCommandBase flags
	c.configBase.SetFlags(f)

	// Set the --format and -o flags
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": c.FormatYaml,
		"json": c.FormatJson,
	})

	if featureflag.Enabled(feature.Branches) || featureflag.Enabled(feature.Generations) {
		f.StringVar(&c.branchName, "branch", "", "Specifically target config for the supplied branch")
	}
}

// getAPI either uses the fake API set at test time or that is nil, gets a real
// API and sets that as the API.
func (c *configCommand) getAPI() (ApplicationAPI, error) {
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

	if err := c.validateGeneration(); err != nil {
		return errors.Trace(err)
	}

	c.applicationName = args[0]
	return c.configBase.Init(args[1:])
}

func (c *configCommand) validateGeneration() error {
	if c.branchName == "" {
		branchName, err := c.ActiveBranch()
		if err != nil {
			return errors.Trace(err)
		}
		c.branchName = branchName
	}

	// TODO (manadart 2019-02-04): If the generation feature is inactive,
	// we set a default in lieu of empty values. This is an expediency
	// during development. When we remove the flag, there will be tests
	// (particularly feature tests) that will need to accommodate a value
	// for branch in the local store.
	if !featureflag.Enabled(feature.Branches) && !featureflag.Enabled(feature.Generations) && c.branchName == "" {
		c.branchName = model.GenerationMaster
	}

	return nil
}

// Run implements the cmd.Command interface.
func (c *configCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = client.Close() }()

	for _, action := range c.configBase.Actions {
		var err error
		switch action {
		case config.GetOne:
			err = c.getConfig(client, ctx)
		case config.SetArgs:
			err = c.setConfig(client, ctx)
		case config.SetFile:
			err = c.setConfigFile(client, ctx)
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

// resetConfig is the run action when we are resetting attributes.
func (c *configCommand) resetConfig(client ApplicationAPI) error {
	err := client.UnsetApplicationConfig(c.branchName, c.applicationName, c.configBase.KeysToReset)
	return block.ProcessBlockedError(err, block.BlockChange)
}

// setConfig is the run action when we are setting config values from the
// provided command-line arguments.
func (c *configCommand) setConfig(client ApplicationAPI, ctx *cmd.Context) error {
	settings, err := c.validateValues(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	err = client.SetConfig(c.branchName, c.applicationName, "", settings)
	return errors.Trace(block.ProcessBlockedError(err, block.BlockChange))
}

// setConfigFile is the run action when we are setting config values from a
// yaml file.
func (c *configCommand) setConfigFile(client ApplicationAPI, ctx *cmd.Context) error {
	var (
		b   []byte
		err error
	)
	if c.configBase.ConfigFile.Path == "-" {
		buf := bytes.Buffer{}
		if _, err := buf.ReadFrom(ctx.Stdin); err != nil {
			return errors.Trace(err)
		}
		b = buf.Bytes()
	} else {
		b, err = c.configBase.ConfigFile.Read(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}

	err = client.SetConfig(c.branchName, c.applicationName, string(b), map[string]string{})
	return errors.Trace(block.ProcessBlockedError(err, block.BlockChange))
}

// getConfig is the run action to return a single configuration value.
func (c *configCommand) getConfig(client ApplicationAPI, ctx *cmd.Context) error {
	results, err := client.Get(c.branchName, c.applicationName)
	if err != nil {
		return err
	}

	logger.Infof("format %v is ignored", c.out.Name())
	if len(c.configBase.KeysToGet) == 0 {
		return errors.New("c.configBase.KeysToGet is empty")
	}
	key := c.configBase.KeysToGet[0]
	info, found := results.CharmConfig[key].(map[string]interface{})
	if !found && len(results.ApplicationConfig) > 0 {
		info, found = results.ApplicationConfig[key].(map[string]interface{})
	}
	if !found {
		return errors.Errorf("key %q not found in %q application config or charm settings.", key, c.applicationName)
	}
	v, ok := info["value"]
	if !ok || v == nil {
		v = ""
	}
	_, err = fmt.Fprintln(ctx.Stdout, v)
	return errors.Trace(err)
}

// getAllConfig is the run action to return all configuration values.
func (c *configCommand) getAllConfig(client ApplicationAPI, ctx *cmd.Context) error {
	results, err := client.Get(c.branchName, c.applicationName)
	if err != nil {
		return err
	}

	resultsMap := map[string]interface{}{
		"application": results.Application,
		"charm":       results.Charm,
		"settings":    results.CharmConfig,
	}
	if len(results.ApplicationConfig) > 0 {
		resultsMap["application-config"] = results.ApplicationConfig
	}

	err = c.out.Write(ctx, resultsMap)

	if (featureflag.Enabled(feature.Branches) || featureflag.Enabled(feature.Generations)) && err == nil {
		var gen string
		gen, err = c.ActiveBranch()
		if err == nil {
			_, err = ctx.Stdout.Write([]byte(fmt.Sprintf("\nchanges will be targeted to generation: %s\n", gen)))
		}
	}
	return errors.Trace(err)
}

// validateValues reads the values provided as args and validates that they are
// valid UTF-8.
func (c *configCommand) validateValues(ctx *cmd.Context) (map[string]string, error) {
	settings := map[string]string{}
	for k, v := range c.configBase.ValsToSet {
		vStr := fmt.Sprint(v) // `v` is generally a string
		//empty string is also valid as a setting value
		if vStr == "" {
			settings[k] = vStr
			continue
		}

		if vStr[0] != '@' {
			if !utf8.ValidString(vStr) {
				return nil, errors.Errorf("value for option %q contains non-UTF-8 sequences", k)
			}
			settings[k] = vStr
			continue
		}
		nv, err := utils.ReadValue(ctx, c.Filesystem(), vStr[1:])
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

// FormatYaml serializes value into valid yaml string. If color flag is passed it adds ANSI color escape codes to the output.
func (c *configCommand) FormatYaml(w io.Writer, value interface{}) error {
	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") && !c.configBase.Color || c.configBase.NoColor {
		return cmd.FormatYaml(w, value)
	}

	if c.configBase.Color {
		return output.FormatYamlWithColor(w, value)
	}

	if utils.IsTerminal(w) && !c.configBase.NoColor {
		return output.FormatYamlWithColor(w, value)
	}

	if !utils.IsTerminal(w) && c.configBase.Color {
		return output.FormatYamlWithColor(w, value)
	}

	return cmd.FormatYaml(w, value)
}

// FormatJson serializes value into valid json string. If color flag is passed it adds ANSI color escape codes to the output.
func (c *configCommand) FormatJson(w io.Writer, val interface{}) error {
	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") && !c.configBase.Color || c.configBase.NoColor {
		return cmd.FormatJson(w, val)
	}

	if c.configBase.Color {
		return output.FormatJsonWithColor(w, val)
	}

	if utils.IsTerminal(w) && !c.configBase.NoColor {
		return output.FormatJsonWithColor(w, val)
	}

	if !utils.IsTerminal(w) && c.configBase.Color {
		return output.FormatJsonWithColor(w, val)
	}

	return cmd.FormatJson(w, val)
}
