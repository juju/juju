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
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/instance"
)

var usageAddUnitSummary = `
Adds one or more units to a deployed application.`[1:]

var usageAddUnitDetails = `
The add-unit command adds units to an existing application. It is used
to scale out an application for improved performance or availability.

Many charms will seamlessly support horizontal scaling while others
may need an additional application support (e.g. a separate load
balancer). See the documentation for specific charms to check how
scale-out is supported.

By default, units are deployed to newly provisioned machines in
accordance with any application or model constraints. This command
also supports the placement directive ("--to") for targeting specific
machines or containers, which will bypass application and model
constraints.

Examples:

Add five units of wordpress on five new machines:
    juju add-unit wordpress -n 5

Add a unit of mysql to machine 23 (which already exists):
    juju add-unit mysql --to 23

Add two units of mysql to machines 3 and 4:
   juju add-unit mysql -n 2 --to 3,4

Add three units of mysql to machine 7:
    juju add-unit mysql -n 3 --to 7,7,7

Add three units of mysql, one to machine 3 and the others to new
machines:
    juju add-unit mysql -n 3 --to 7

Add a unit into a new LXD container on machine 7:
    juju add-unit mysql --to lxd:7

Add two units into two new LXD containers on machine 7:
    juju add-unit mysql -n 2 --to lxd:7,lxd:7

Add a unit of mariadb to LXD container number 3 on machine 24:
    juju add-unit mariadb --to 24/lxd/3

Add a unit of mariadb to LXD container on a new machine:
    juju add-unit mariadb --to lxd

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
	// AttachStorage is a list of storage IDs, identifying storage to
	// attach to the unit created by deploy.
	AttachStorage []string
}

func (c *UnitCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.NumUnits, "num-units", 1, "")
	f.StringVar(&c.PlacementSpec, "to", "", "The machine and/or container to deploy the unit in (bypasses constraints)")
	f.Var(attachStorageFlag{&c.AttachStorage}, "attach-storage", "Existing storage to attach to the deployed unit")
}

func (c *UnitCommandBase) Init(args []string) error {
	if c.NumUnits < 1 {
		return errors.New("--num-units must be a positive integer")
	}
	if len(c.AttachStorage) > 0 && c.NumUnits != 1 {
		return errors.New("--attach-storage cannot be used with -n")
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
	api             applicationAddUnitAPI
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

// applicationAddUnitAPI defines the methods on the client API
// that the application add-unit command calls.
type applicationAddUnitAPI interface {
	BestAPIVersion() int
	Close() error
	ModelUUID() string
	AddUnits(application.AddUnitsParams) ([]string, error)
}

func (c *addUnitCommand) getAPI() (applicationAddUnitAPI, error) {
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
func (c *addUnitCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	if len(c.AttachStorage) > 0 && apiclient.BestAPIVersion() < 5 {
		// AddUnitsPArams.AttachStorage is only supported from
		// Application API version 5 and onwards.
		return errors.New("this juju controller does not support --attach-storage")
	}

	for i, p := range c.Placement {
		if p.Scope == "model-uuid" {
			p.Scope = apiclient.ModelUUID()
		}
		c.Placement[i] = p
	}
	_, err = apiclient.AddUnits(application.AddUnitsParams{
		ApplicationName: c.ApplicationName,
		NumUnits:        c.NumUnits,
		Placement:       c.Placement,
		AttachStorage:   c.AttachStorage,
	})
	if params.IsCodeUnauthorized(err) {
		common.PermissionsMessage(ctx.Stderr, "add a unit")
	}
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
