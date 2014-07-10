// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// SetCommand updates the configuration of a service.
type SetCommand struct {
	envcmd.EnvCommandBase
	ServiceName     string
	SettingsStrings map[string]string
	SettingsYAML    cmd.FileVar
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
		Doc:     "Set one or more configuration options for the specified service.",
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
	settings, err := parse(args[1:])
	if err != nil {
		return err
	}
	c.SettingsStrings = settings
	return nil
}

// Run updates the configuration of a service.
func (c *SetCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	if c.SettingsYAML.Path != "" {
		b, err := c.SettingsYAML.Read(ctx)
		if err != nil {
			return err
		}
		return api.ServiceSetYAML(c.ServiceName, string(b))
	} else if len(c.SettingsStrings) == 0 {
		return nil
	}
	settings := map[string]string{}
	for k, v := range c.SettingsStrings {
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
	return api.ServiceSet(c.ServiceName, settings)
}

// parse parses the option k=v strings into a map of options to be
// updated in the config. Keys with empty values are returned separately
// and should be removed.
func parse(options []string) (map[string]string, error) {
	kv := make(map[string]string)
	for _, o := range options {
		s := strings.SplitN(o, "=", 2)
		if len(s) != 2 || s[0] == "" {
			return nil, fmt.Errorf("invalid option: %q", o)
		}
		kv[s[0]] = s[1]
	}
	return kv, nil
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
