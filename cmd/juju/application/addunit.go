// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/instance"
)

var usageAddUnitSummary = `
Adds one or more units to a deployed application.`[1:]

var usageAddUnitDetails = `
Adding units to an existing application is a way to scale out that application. 
Many charms will seamlessly support horizontal scaling, others may need an
additional application to facilitate load-balancing (check the individual 
charm documentation).
This command is applied to applications that have already been deployed.
By default, applications are deployed to newly provisioned machines in
accordance with any application or model constraints. Alternatively, this 
command also supports the placement directive ("--to") for targeting
specific machines or containers, which will bypass any existing
constraints.

Examples:
Add five units of wordpress on five new machines:

    juju add-unit wordpress -n 5

Add one unit of mysql to the existing machine 23:

    juju add-unit mysql --to 23

Create a new LXD container on machine 7 and add one unit of mysql:

    juju add-unit mysql --to lxd:7

Add a unit of mariadb to LXD container number 3 on machine 24:

    juju add-unit mariadb --to 24/lxd/3

See also: 
    remove-unit`[1:]

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
	f.StringVar(&c.PlacementSpec, "to", "", "The machine and/or container to deploy the unit in (bypasses constraints)")
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

// NewAddUnitCommand returns a command that adds a unit[s] to an application.
func NewAddUnitCommand() cmd.Command {
	return modelcmd.Wrap(&addUnitCommand{})
}

// addUnitCommand is responsible adding additional units to an application.
type addUnitCommand struct {
	modelcmd.ModelCommandBase
	UnitCommandBase
	ApplicationName string
	api             serviceAddUnitAPI
}

func (c *addUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-unit",
		Args:    "<application name>",
		Purpose: usageAddUnitSummary,
		Doc:     usageAddUnitDetails,
	}
}

func (c *addUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.UnitCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "Number of units to add")
}

func (c *addUnitCommand) Init(args []string) error {
	switch len(args) {
	case 1:
		c.ApplicationName = args[0]
	case 0:
		return errors.New("no application specified")
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return err
	}
	return c.UnitCommandBase.Init(args)
}

// serviceAddUnitAPI defines the methods on the client API
// that the application add-unit command calls.
type serviceAddUnitAPI interface {
	Close() error
	ModelUUID() string
	AddUnits(application string, numUnits int, placement []*instance.Placement) ([]string, error)
}

func (c *addUnitCommand) getAPI() (serviceAddUnitAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

// Run connects to the environment specified on the command line
// and calls AddUnits for the given application.
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
	_, err = apiclient.AddUnits(c.ApplicationName, c.NumUnits, c.Placement)
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
