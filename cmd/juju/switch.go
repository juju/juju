// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"sort"

	"launchpad.net/gnuflag"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
)

type SwitchCommand struct {
	cmd.CommandBase
	out     cmd.Output
	EnvName string
	List    bool
}

var switchDoc = `
Show or change the default juju environment name.

If no command line parameters are passed, switch will output the current
environment as defined by the file $JUJU_HOME/current-environment.

If a command line parameter is passed in, that value will is stored in the
current environment file if it represents a valid environment name as
specified in the environments.yaml file.

Use the --format flag to print out information about the environment. You 
need to specify a YAML or JSON format: "--format yaml" or "--format json".

Examples:

# Show current environment name
$ juju switch
local

# Switch to the ec2 environment. Show the environment you are switching
# from and to i.e. "oldEnv -> newEnv"
$ juju switch ec2
local -> ec2 

# List all available environments to switch to
$ juju switch --list
amazon
azure
hpcloud
joyent
local
maas
manual
openstack

# List all available environments to switch to
$ juju switch -l
amazon
azure
hpcloud
joyent
local
maas
manual
openstack

# Show username, API endpoints and environment name
$ juju switch --format yaml
environ-name: local
state-servers:
	- example.com
	- kremvax.ru
username: joe

# Show infomation for the environment you are switching to. Include 
# the environment you are switching from in the 'previous-environ-name' field.
$ juju switch local --format yaml
environ-name: local
previous-environ-name: ec2
state-servers:
	- example.com
	- kremvax.ru
username: joe

# Format environment information as json
$ juju switch --format json
{ "state-servers": ["example.com","kremvax.ru"],
"environ-name":"local",
"username":"joe" }
`

type EnvInfo struct {
	Username            string   `yaml:"user-name" json:"user-name"`
	EnvironName         string   `yaml:"environ-name" json:"environ-name"`
	PreviousEnvironName string   `yaml:"previous-environ-name,omitempty" json:"previous-environ-name,omitempty"`
	StateServers        []string `yaml:"state-servers,omitempty" json:"state-servers,omitempty"`
}

func (c *SwitchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "switch",
		Args:    "[environment name]",
		Purpose: "show or change the default juju environment name",
		Doc:     switchDoc,
		Aliases: []string{"env"},
	}
}

func (c *SwitchCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.List, "l", false, "list the environment names")
	f.BoolVar(&c.List, "list", false, "")
	c.out.AddFlags(f, "simple", map[string]cmd.Formatter{
		"simple": c.formatSimple,
		"yaml":   cmd.FormatYaml,
		"json":   cmd.FormatJson,
	})
}

func (c *SwitchCommand) Init(args []string) (err error) {
	c.EnvName, err = cmd.ZeroOrOneArgs(args)
	return
}

func validEnvironmentName(name string, names []string) bool {
	for _, n := range names {
		if name == n {
			return true
		}
	}
	return false
}

func (c *SwitchCommand) Run(ctx *cmd.Context) error {
	// Switch is an alternative way of dealing with environments than using
	// the JUJU_ENV environment setting, and as such, doesn't play too well.
	// If JUJU_ENV is set we should report that as the current environment,
	// and not allow switching when it is set.

	// Passing through the empty string reads the default environments.yaml file.
	environments, err := environs.ReadEnvirons("")
	if err != nil {
		return errors.New("couldn't read the environment")
	}
	names := environments.Names()
	sort.Strings(names)

	if c.List {
		if c.EnvName != "" {
			return errors.New("cannot switch and list at the same time")
		}
		for _, name := range names {
			fmt.Fprintf(ctx.Stdout, "%s\n", name)
		}
		return nil
	}

	var info EnvInfo

	jujuEnv := os.Getenv("JUJU_ENV")
	if jujuEnv != "" {
		if c.EnvName == "" {
			if info, err = buildEnvInfo(jujuEnv, ""); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("cannot switch when JUJU_ENV is overriding the environment (set to %q)", jujuEnv)
		}
	} else {
		currentEnv := envcmd.ReadCurrentEnvironment()
		if currentEnv == "" {
			currentEnv = environments.Default
		}

		// Handle the different operation modes.
		switch {
		case c.EnvName == "" && currentEnv == "":
			// Nothing specified and nothing to switch to.
			return errors.New("no currently specified environment")
		case c.EnvName == "":
			// Simply print the current environment.
			if info, err = buildEnvInfo(currentEnv, ""); err != nil {
				return err
			}
		default:
			// Switch the environment.
			if !validEnvironmentName(c.EnvName, names) {
				return fmt.Errorf("%q is not a name of an existing defined environment", c.EnvName)
			}
			if err := envcmd.WriteCurrentEnvironment(c.EnvName); err != nil {
				return err
			}
			if info, err = buildEnvInfo(c.EnvName, currentEnv); err != nil {
				return err
			}
		}
	}

	if err = c.out.Write(ctx, info); err != nil {
		return err
	}

	return nil
}

// buildEnvInfo builds and returns an EnvInfo
func buildEnvInfo(envName string, oldEnvName string) (info EnvInfo, err error) {
	store, err := configstore.Default()
	if err != nil {
		return EnvInfo{}, err
	}
	cfgEnvInfo, err := store.ReadInfo(envName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return EnvInfo{}, err
		}
	} else {
		info.Username = cfgEnvInfo.APICredentials().User
		info.StateServers = cfgEnvInfo.APIEndpoint().Addresses
	}
	info.EnvironName = envName
	info.PreviousEnvironName = oldEnvName
	return info, nil
}

func (c *SwitchCommand) formatSimple(value interface{}) (output []byte, err error) {
	if info, ok := value.(EnvInfo); ok {
		var msg string
		if info.PreviousEnvironName != "" {
			msg = fmt.Sprintf("%s -> %s", info.PreviousEnvironName, info.EnvironName)
		} else {
			var fmtStr string
			if c.EnvName != "" {
				fmtStr = "-> %s"
			} else {
				fmtStr = "%s"
			}
			msg = fmt.Sprintf(fmtStr, info.EnvironName)
		}
		output = []byte(msg)
		return output, nil
	}
	return nil, err
}
