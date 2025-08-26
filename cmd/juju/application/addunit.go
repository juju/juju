// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"regexp"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

var usageAddUnitSummary = `Adds one or more units to a deployed application.`

var usageAddUnitDetails = `
The ` + "`add-unit`" + ` command is used to scale out an application for improved performance or
availability.

Note: Some charms will seamlessly support horizontal scaling while others may need
an additional application support (e.g. a separate load balancer). See the
documentation for specific charms to check how scale-out is supported.

Further reading:

- https://documentation.ubuntu.com/juju/3.6/reference/unit/
- https://documentation.ubuntu.com/juju/3.6/reference/placement-directive/

`[1:]

const usageAddUnitExamples = `
Add five units of mysql on five new machines:

    juju add-unit mysql -n 5

Add a unit of mysql to machine 23 (which already exists):

    juju add-unit mysql --to 23

Add two units of mysql to existing machines 3 and 4:

    juju add-unit mysql -n 2 --to 3,4

Add three units of mysql, one to machine 3 and the others to new
machines:

    juju add-unit mysql -n 3 --to 3

Add a unit of mysql into a new LXD container on machine 7:

    juju add-unit mysql --to lxd:7

Add two units of mysql into two new LXD containers on machine 7:

    juju add-unit mysql -n 2 --to lxd:7,lxd:7

Add three units of mysql, one to a new LXD container on machine 7,
and the others to new machines:

    juju add-unit mysql -n 3 --to lxd:7

Add a unit of mysql to LXD container number 3 on machine 24:

    juju add-unit mysql --to 24/lxd/3

Add a unit of mysql to LXD container on a new machine:

    juju add-unit mysql --to lxd
`

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
	f.StringVar(&c.PlacementSpec, "to", "", "(Machine models only) Specify a comma-separated list of placement directives. If the length of this list is less than `-n`, the remaining units will be added in the default way (i.e., to new machines).")
	f.Var(attachStorageFlag{&c.AttachStorage}, "attach-storage", "(Machine models only) Specify an existing storage volume to attach to the deployed unit.")
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
		// Ensure that Placement length is accurate, wait for valid placements
		// to add.
		c.Placement = make([]*instance.Placement, 0)
		for _, spec := range placementSpecs {
			if spec == "" {
				return errors.Errorf("invalid --to parameter %q", c.PlacementSpec)
			}
			placement, err := utils.ParsePlacement(spec)
			if err != nil {
				return errors.Errorf("invalid --to parameter %q", spec)
			}
			c.Placement = append(c.Placement, placement)
		}
	}
	if len(c.Placement) > c.NumUnits {
		logger.Warningf("%d unit(s) will be deployed, extra placement directives will be ignored", c.NumUnits)
	}
	return nil
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

	unknownModel bool
}

func (c *addUnitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-unit",
		Args:     "<application name>",
		Purpose:  usageAddUnitSummary,
		Doc:      usageAddUnitDetails,
		Examples: usageAddUnitExamples,
		SeeAlso: []string{
			"remove-unit",
		},
	})
}

func (c *addUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.UnitCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "Specify the number of units to add.")
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
	if err := c.validateArgsByModelType(); err != nil {
		if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		c.unknownModel = true
	}

	return c.UnitCommandBase.Init(args)
}

func (c *addUnitCommand) validateArgsByModelType() error {
	modelType, err := c.ModelType()
	if err != nil {
		return err
	}
	if modelType == model.CAAS {
		if c.PlacementSpec != "" || len(c.AttachStorage) != 0 {
			return errors.New("k8s models only support --num-units")
		}
	}
	return nil
}

// applicationAddUnitAPI defines the methods on the client API
// that the application add-unit command calls.
type applicationAddUnitAPI interface {
	Close() error
	ModelUUID() string
	AddUnits(application.AddUnitsParams) ([]string, error)
	ScaleApplication(application.ScaleApplicationParams) (params.ScaleApplicationResult, error)
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

	if c.unknownModel {
		if err := c.validateArgsByModelType(); err != nil {
			return errors.Trace(err)
		}
	}

	modelType, err := c.ModelType()
	if err != nil {
		return err
	}

	if modelType == model.CAAS {
		_, err = apiclient.ScaleApplication(application.ScaleApplicationParams{
			ApplicationName: c.ApplicationName,
			ScaleChange:     c.NumUnits,
		})
		if err == nil {
			return nil
		}
		if params.IsCodeNotSupported(err) {
			return errors.Annotate(err, "can not add unit")
		}
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "scale an application")
		}
		return block.ProcessBlockedError(err, block.BlockChange)
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
