// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/charmstore"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd"
)

// ConfigCommand defines a command which requires a YAML config file.
type ConfigCommand struct {
	cmd.CommandBase
	ConfigPath string
	Config     *charmstore.Config
}

type CharmdConfig struct {
	MongoUrl string `yaml:"mongo-url"`
}

func (c *ConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.ConfigPath, "config", "", "charmd configuration file")
}

func (c *ConfigCommand) Init(args []string) error {
	if c.ConfigPath == "" {
		return fmt.Errorf("--config is required")
	}
	return nil
}

func (c *ConfigCommand) ReadConfig(ctx *cmd.Context) (err error) {
	c.Config, err = charmstore.ReadConfig(ctx.AbsPath(c.ConfigPath))
	return err
}
