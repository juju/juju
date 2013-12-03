// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/cmd"
)

// ConfigCommand defines a command which requires a YAML config file.
type ConfigCommand struct {
	cmd.CommandBase
	ConfigPath string
}

func (c *ConfigCommand) Run(ctx *cmd.Context) error {
	if c.ConfigPath == "" {
		return fmt.Errorf("--config is required")
	}
	return nil
}

func (c *ConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.ConfigPath, "config", "", "charmd configuration file")
}

// ReadConfig reads the YAML config file into the given interface,
// which can be the address of a struct or a map.
func (c *ConfigCommand) ReadConfig(conf interface{}) error {
	f, err := os.Open(c.ConfigPath)
	if err != nil {
		return fmt.Errorf("opening config file: %v", err)
	}
	data, err := ioutil.ReadAll(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("reading config file: %v", err)
	}
	err = goyaml.Unmarshal(data, conf)
	if err != nil {
		return fmt.Errorf("processing config file: %v", err)
	}
	return nil
}
