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

	apiservice "github.com/juju/juju/api/service"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/instance"
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
	if spec == "" {
		return nil, nil
	}
	placement, err := instance.ParsePlacement(spec)
	if err == instance.ErrPlacementScopeMissing {
		spec = "model-uuid" + ":" + spec
		placement, err = instance.ParsePlacement(spec)
	}
	if err != nil {
		return nil, errors.Errorf("invalid --to parameter %q", spec)
	}
	return placement, nil
}

// NewAddUnitCommand returns a command that adds a unit[s] to a service.
func NewAddUnitCommand() cmd.Command {
	return modelcmd.Wrap(&addUnitCommand{})
}

// addUnitCommand is responsible adding additional units to a service.
type addUnitCommand struct {
	modelcmd.ModelCommandBase
	UnitCommandBase
	ServiceName string
	api         serviceAddUnitAPI
}

const addUnitDoc = `
Adding units to an existing service is a way to scale out a model by
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

func (c *addUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-unit",
		Args:    "<service name>",
		Purpose: "add one or more units of an already-deployed service",
		Doc:     addUnitDoc,
		Aliases: []string{"add-units"},
	}
}

func (c *addUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.UnitCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to add")
}

func (c *addUnitCommand) Init(args []string) error {
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

// serviceAddUnitAPI defines the methods on the client API
// that the service add-unit command calls.
type serviceAddUnitAPI interface {
	Close() error
	ModelUUID() string
	AddUnits(service string, numUnits int, placement []*instance.Placement) ([]string, error)
}

func (c *addUnitCommand) getAPI() (serviceAddUnitAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiservice.NewClient(root), nil
}

// Run connects to the environment specified on the command line
// and calls AddUnits for the given service.
func (c *addUnitCommand) Run(_ *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	for i, p := range c.Placement {
		if p.Scope == "model-uuid" {
			p.Scope = apiclient.ModelUUID()
		}
		c.Placement[i] = p
	}
	_, err = apiclient.AddUnits(c.ServiceName, c.NumUnits, c.Placement)
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
