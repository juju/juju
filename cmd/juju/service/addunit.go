// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider"
)

// UnitCommandBase provides support for commands which deploy units. It handles the parsing
// and validation of --to and --num-units arguments.
type UnitCommandBase struct {
	ToMachineSpec string
	NumUnits      int
}

func (c *UnitCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.NumUnits, "num-units", 1, "")
	f.StringVar(&c.ToMachineSpec, "to", "", "the machine or container to deploy the unit in, bypasses constraints")
}

func (c *UnitCommandBase) Init(args []string) error {
	if c.NumUnits < 1 {
		return errors.New("--num-units must be a positive integer")
	}
	if c.ToMachineSpec != "" {
		if c.NumUnits > 1 {
			return errors.New("cannot use --num-units > 1 with --to")
		}
		if !IsMachineOrNewContainer(c.ToMachineSpec) {
			return fmt.Errorf("invalid --to parameter %q", c.ToMachineSpec)
		}

	}
	return nil
}

// TODO(anastasiamac) 2014-10-20 Bug#1383116
// This exists to provide more context to the user about
// why they cannot allocate units to machine 0. Remove
// this when the local provider's machine 0 is a container.
// TODO(cherylj) Unexport CheckProvider once deploy is moved under service
func (c *UnitCommandBase) CheckProvider(conf *config.Config) error {
	if conf.Type() == provider.Local && c.ToMachineSpec == "0" {
		return errors.New("machine 0 is the state server for a local environment and cannot host units")
	}
	return nil
}

// TODO(cherylj) Unexport GetClientConfig once deploy is moved under service
var GetClientConfig = func(client ServiceAddUnitAPI) (*config.Config, error) {
	// Separated into a variable for easy overrides
	attrs, err := client.EnvironmentGet()
	if err != nil {
		return nil, err
	}

	return config.New(config.NoDefaults, attrs)
}

// AddUnitCommand is responsible adding additional units to a service.
type AddUnitCommand struct {
	envcmd.EnvCommandBase
	UnitCommandBase
	ServiceName string
	api         ServiceAddUnitAPI
}

const addUnitDoc = `
Adding units to an existing service is a way to scale out an environment by
deploying more instances of a service.  Add-unit must be called on services that
have already been deployed via juju deploy.  

By default, services are deployed to newly provisioned machines.  Alternatively,
service units can be added to a specific existing machine using the --to
argument.

Examples:
 juju add-unit mysql -n 5          (Add 5 mysql units on 5 new machines)
 juju add-unit mysql --to 23       (Add a mysql unit to machine 23)
 juju add-unit mysql --to 24/lxc/3 (Add unit to lxc container 3 on host machine 24)
 juju add-unit mysql --to lxc:25   (Add unit to a new lxc container on host machine 25)
`

func (c *AddUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-unit",
		Args:    "<service name>",
		Purpose: "add one or more units of an already-deployed service",
		Doc:     addUnitDoc,
	}
}

func (c *AddUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.UnitCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to add")
}

func (c *AddUnitCommand) Init(args []string) error {
	switch len(args) {
	case 1:
		c.ServiceName = args[0]
	case 0:
		return errors.New("no service specified")
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return err
	}
	return c.UnitCommandBase.Init(args)
}

// ServiceAddUnitAPI defines the methods on the client API
// that the service add-unit command calls.
type ServiceAddUnitAPI interface {
	Close() error
	AddServiceUnits(service string, numUnits int, machineSpec string) ([]string, error)
	EnvironmentGet() (map[string]interface{}, error)
}

func (c *AddUnitCommand) getAPI() (ServiceAddUnitAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run connects to the environment specified on the command line
// and calls AddServiceUnits for the given service.
func (c *AddUnitCommand) Run(_ *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	conf, err := GetClientConfig(apiclient)
	if err != nil {
		return err
	}

	if err := c.CheckProvider(conf); err != nil {
		return err
	}

	_, err = apiclient.AddServiceUnits(c.ServiceName, c.NumUnits, c.ToMachineSpec)
	return block.ProcessBlockedError(err, block.BlockChange)
}

const (
	deployTarget = "^(" + names.ContainerTypeSnippet + ":)?" + names.MachineSnippet + "$"
)

var (
	validMachineOrNewContainer = regexp.MustCompile(deployTarget)
)

// IsMachineOrNewContainer returns whether spec is a valid machine id
// or new container definition.
func IsMachineOrNewContainer(spec string) bool {
	return validMachineOrNewContainer.MatchString(spec)
}
