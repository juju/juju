// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider"
)

// UnitCommandBase provides support for commands which deploy units. It handles the parsing
// and validation of --to and --num-units arguments.
type UnitCommandBase struct {
	// PlacementSpec is the raw string command arg value used to specify placement directives.
	PlacementSpec string
	// Placement is the result of parsing the PlacementSpec arg value.
	Placement []*instance.Placement
	NumUnits  int
}

func (c *UnitCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.NumUnits, "num-units", 1, "")
	f.StringVar(&c.PlacementSpec, "to", "", "the machine, container or placement directive to deploy the unit in, bypasses constraints")
}

func (c *UnitCommandBase) Init(args []string) error {
	if c.NumUnits < 1 {
		return errors.New("--num-units must be a positive integer")
	}
	if c.PlacementSpec != "" {
		// Older Juju versions just accept a single machine or container.
		if IsMachineOrNewContainer(c.PlacementSpec) {
			return nil
		}
		// Newer Juju versions accept a comma separated list of placement directives.
		placementSpecs := strings.Split(c.PlacementSpec, ",")
		c.Placement = make([]*instance.Placement, len(placementSpecs))
		for i, spec := range placementSpecs {
			placement, err := parsePlacement(spec)
			if err != nil {
				return errors.Errorf("invalid --to parameter %q", spec)
			}
			c.Placement[i] = placement
		}
	}
	if len(c.Placement) > c.NumUnits {
		logger.Warningf("%d unit(s) will be deployed, extra placement directives will be ignored", c.NumUnits)
	}
	return nil
}

func parsePlacement(spec string) (*instance.Placement, error) {
	placement, err := instance.ParsePlacement(spec)
	if err == instance.ErrPlacementScopeMissing {
		spec = "env-uuid" + ":" + spec
		placement, err = instance.ParsePlacement(spec)
	}
	if err != nil {
		return nil, errors.Errorf("invalid --to parameter %q", spec)
	}
	return placement, nil
}

// TODO(anastasiamac) 2014-10-20 Bug#1383116
// This exists to provide more context to the user about
// why they cannot allocate units to machine 0. Remove
// this when the local provider's machine 0 is a container.
// TODO(cherylj) Unexport CheckProvider once deploy is moved under service
func (c *UnitCommandBase) CheckProvider(conf *config.Config) error {
	isMachineZero := c.PlacementSpec == "0"
	for _, p := range c.Placement {
		isMachineZero = isMachineZero || (p.Scope == instance.MachineScope && p.Directive == "0")
	}
	if conf.Type() == provider.Local && isMachineZero {
		return errors.New("machine 0 is the state server for a local environment and cannot host units")
	}
	return nil
}

// TODO(cherylj) Unexport GetClientConfig and make it a standard function
// once deploy is moved under service
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
 juju service add-unit mysql -n 5          (Add 5 mysql units on 5 new machines)
 juju service add-unit mysql --to 23       (Add a mysql unit to machine 23)
 juju service add-unit mysql --to 24/lxc/3 (Add unit to lxc container 3 on host machine 24)
 juju service add-unit mysql --to lxc:25   (Add unit to a new lxc container on host machine 25)
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
	EnvironmentUUID() string
	AddServiceUnits(service string, numUnits int, machineSpec string) ([]string, error)
	AddServiceUnitsWithPlacement(service string, numUnits int, placement []*instance.Placement) ([]string, error)
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

	for i, p := range c.Placement {
		if p.Scope == "env-uuid" {
			p.Scope = apiclient.EnvironmentUUID()
		}
		c.Placement[i] = p
	}
	if len(c.Placement) > 0 {
		_, err = apiclient.AddServiceUnitsWithPlacement(c.ServiceName, c.NumUnits, c.Placement)
		if err == nil {
			return nil
		}
		if !params.IsCodeNotImplemented(err) {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
	}
	if c.PlacementSpec != "" && !IsMachineOrNewContainer(c.PlacementSpec) {
		return errors.Errorf("unsupported --to parameter %q", c.PlacementSpec)
	}
	if c.PlacementSpec != "" && c.NumUnits > 1 {
		return errors.New("this version of Juju does not support --num-units > 1 with --to")
	}
	_, err = apiclient.AddServiceUnits(c.ServiceName, c.NumUnits, c.PlacementSpec)
	return block.ProcessBlockedError(err, block.BlockChange)
}

// deployTarget describes the format a machine or container target must match to be valid.
const deployTarget = "^(" + names.ContainerTypeSnippet + ":)?" + names.MachineSnippet + "$"

var validMachineOrNewContainer = regexp.MustCompile(deployTarget)

// IsMachineOrNewContainer returns whether spec is a valid machine id
// or new container definition.
func IsMachineOrNewContainer(spec string) bool {
	return validMachineOrNewContainer.MatchString(spec)
}
