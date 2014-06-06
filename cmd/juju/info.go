// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"sort"

	"launchpad.net/gnuflag"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
)

type InfoCommand struct {
	envcmd.EnvCommandBase
	out  cmd.Output
	List bool
	All  bool
}

var infoDoc = `
Print out information about an environment. Use the 
--format=[yaml,json] to format output.

Examples:

# Show information for the current environment, formatted in YAML
# by default
$ juju info
environ-name: local
username: joe

# Show information about a different environment by specifing a 
# name on the command line
$ juju info -e amazon
environ-name: amazon
username: joe

#  Use the the --all flag to display extra information, namely:
# API endpoints (UUID when it becomes available)
$ juju info -e local --all
environ-name: local
username: joe
state-servers:
- localhost:17070
- 10.0.3.1:17070
status: running

#  Use "--format json" to output information in JSON
$ juju info -e amazon --all --format json
"{"environment-name":"local","user-name":"joe","status":"running","api-endpoints":["localhost:17070","10.0.3.1:17070"]}"

# List all available environments
$ juju info --list
amazon
azure
hpcloud
joyent
local
maas
manual
openstack


# List all available environments using the short flag -l
$ juju info -l
amazon
azure
...


# List all information on all available environments in JSON
$ juju info --list --all --format json
[{"environment-name":"enva","status":"running"},
{"environment-name":"erewhemos","status":"not running"},
{"environment-name":"erewhemos-2","status":"not running"}]
`

type EnvInfo struct {
	EnvironName  string   `yaml:"environment-name" json:"environment-name"`
	Username     string   `yaml:"user-name,omitempty" json:"user-name,omitempty"`
	Status       string   `yaml:"status" json:"status"`
	APIEndpoints []string `yaml:"api-endpoints,omitempty" json:"api-endpoints,omitempty"`
	// TODO(waigani) UUID
}

func (c *InfoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "info",
		Purpose: "show or change the default juju environment name",
		Doc:     infoDoc,
		Aliases: []string{"env"},
	}
}

func (c *InfoCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.List, "l", false, "list the environment names")
	f.BoolVar(&c.List, "list", false, "")
	f.BoolVar(&c.All, "a", false, "Show all environment information")
	f.BoolVar(&c.All, "all", false, "")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *InfoCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

func (c *InfoCommand) Run(ctx *cmd.Context) error {
	// Passing through the empty string reads the default environments.yaml file.
	environments, err := environs.ReadEnvirons("")
	if err != nil {
		return errors.New("couldn't read the environment")
	}
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	envYamlNames := environments.Names()
	jenvNames, err := store.List()
	if err != nil {
		return err
	}

	// merge environment names from environments.yaml and jenvs
	namesMap := map[string]bool{}
	var names []string
	for _, name := range jenvNames {
		namesMap[name] = true
	}
	for _, name := range envYamlNames {
		namesMap[name] = true
	}
	for name, _ := range namesMap {
		names = append(names, name)
	}
	sort.Strings(names)

	if c.List {
		if c.All {
			envs := []EnvInfo{}
			for _, name := range names {
				env, err := c.buildEnvInfo(name, names)
				if err != nil {
					return err
				}
				envs = append(envs, env)
			}
			if c.out.Write(ctx, envs); err != nil {
				return err
			}
		} else {
			for _, name := range names {
				fmt.Fprintf(ctx.Stdout, "%s\n", name)
			}
		}
		return nil
	}

	var info EnvInfo
	if info, err = c.buildEnvInfo(c.EnvName, names); err != nil {
		return err
	}
	if err = c.out.Write(ctx, info); err != nil {
		return err
	}
	return nil
}

// buildEnvInfo builds and returns an EnvInfo
func (s *InfoCommand) buildEnvInfo(envName string, names []string) (info EnvInfo, err error) {
	store, err := configstore.Default()
	if err != nil {
		return EnvInfo{}, err
	}
	cfgEnvInfo, err := store.ReadInfo(envName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return EnvInfo{}, err
		}
		if validEnvironmentName(envName, names) {
			info.Status = "not running"
		} else {
			info.Status = "unknown"
		}

	} else {
		info.Username = cfgEnvInfo.APICredentials().User
		if s.All {
			info.APIEndpoints = cfgEnvInfo.APIEndpoint().Addresses
		}
		info.Status = "running"
	}
	info.EnvironName = envName
	return info, nil
}
