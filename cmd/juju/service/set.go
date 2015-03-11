// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/juju/cmd"
	"github.com/juju/utils/keyvalues"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

// SetCommand updates the configuration of a service.
type SetCommand struct {
	envcmd.EnvCommandBase
	ServiceName     string
	SettingsStrings map[string]string
	SettingsYAML    cmd.FileVar
	api             SetServiceAPI
}

const setDoc = `
Set one or more configuration options for the specified service. See also the
unset command which sets one or more configuration options for a specified
service to their default value.

In case a value starts with an at sign (@) the rest of the value is interpreted
as a filename. The value itself is then read out of the named file. The maximum
size of this value is 5M.

Option values may be any UTF-8 encoded string. UTF-8 is accepted on the command
line and in configuration files.
`

const maxValueSize = 5242880

func (c *SetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set",
		Args:    "<service> name=value ...",
		Purpose: "set service config options",
		Doc:     setDoc,
	}
}

func (c *SetCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.SettingsYAML, "config", "path to yaml-formatted service config")
}

func (c *SetCommand) Init(args []string) error {
	if len(args) == 0 || len(strings.Split(args[0], "=")) > 1 {
		return errors.New("no service name specified")
	}
	if c.SettingsYAML.Path != "" && len(args) > 1 {
		return errors.New("cannot specify --config when using key=value arguments")
	}
	c.ServiceName = args[0]
	settings, err := keyvalues.Parse(args[1:], true)
	if err != nil {
		return err
	}
	c.SettingsStrings = settings
	return nil
}

// SetServiceAPI defines the methods on the client API
// that the service set command calls.
type SetServiceAPI interface {
	Close() error
	ServiceSetYAML(service string, yaml string) error
	ServiceGet(service string) (*params.ServiceGetResults, error)
	ServiceSet(service string, options map[string]string) error
}

func (c *SetCommand) getAPI() (SetServiceAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run updates the configuration of a service.
func (c *SetCommand) Run(ctx *cmd.Context) error {
	api, err := c.getAPI()
	if err != nil {
		return err
	}
	defer api.Close()

	if c.SettingsYAML.Path != "" {
		b, err := c.SettingsYAML.Read(ctx)
		if err != nil {
			return err
		}
		return block.ProcessBlockedError(api.ServiceSetYAML(c.ServiceName, string(b)), block.BlockChange)
	} else if len(c.SettingsStrings) == 0 {
		return nil
	}
	settings := map[string]string{}
	for k, v := range c.SettingsStrings {
		//empty string is also valid as a setting value
		if v == "" {
			settings[k] = v
			continue
		}

		if v[0] != '@' {
			if !utf8.ValidString(v) {
				return fmt.Errorf("value for option %q contains non-UTF-8 sequences", k)
			}
			settings[k] = v
			continue
		}
		nv, err := readValue(ctx, v[1:])
		if err != nil {
			return err
		}
		if !utf8.ValidString(nv) {
			return fmt.Errorf("value for option %q contains non-UTF-8 sequences", k)
		}
		settings[k] = nv
	}

	result, err := api.ServiceGet(c.ServiceName)
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

	return block.ProcessBlockedError(api.ServiceSet(c.ServiceName, settings), block.BlockChange)
}

// readValue reads the value of an option out of the named file.
// An empty content is valid, like in parsing the options. The upper
// size is 5M.
func readValue(ctx *cmd.Context, filename string) (string, error) {
	absFilename := ctx.AbsPath(filename)
	fi, err := os.Stat(absFilename)
	if err != nil {
		return "", fmt.Errorf("cannot read option from file %q: %v", filename, err)
	}
	if fi.Size() > maxValueSize {
		return "", fmt.Errorf("size of option file is larger than 5M")
	}
	content, err := ioutil.ReadFile(ctx.AbsPath(filename))
	if err != nil {
		return "", fmt.Errorf("cannot read option from file %q: %v", filename, err)
	}
	return string(content), nil
}
