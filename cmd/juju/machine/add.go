// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
)

var addMachineDoc = `
Juju supports adding machines using provider-specific machine instances
(EC2 instances, OpenStack servers, MAAS nodes, etc.); existing machines
running a supported operating system (see "manual provisioning" below),
and containers on machines. Machines are created in a clean state and
ready to have units deployed.

Without any parameters, add machine will allocate a new provider-specific
machine (multiple, if "-n" is provided). When adding a new machine, you
may specify constraints for the machine to be provisioned; the provider
will interpret these constraints in order to decide what kind of machine
to allocate.

If a container type is specified (e.g. "lxd"), then add machine will
allocate a container of that type on a new provider-specific machine. It is
also possible to add containers to existing machines using the format
<container type>:<machine number>. Constraints cannot be combined with
deploying a container to an existing machine. The currently supported
container types are: $CONTAINER_TYPES$.

Manual provisioning is the process of installing Juju on an existing machine
and bringing it under Juju's management; currently this requires that the
machine be running Ubuntu, that it be accessible via SSH, and be running on
the same network as the API server.

It is possible to override or augment constraints by passing provider-specific
"placement directives" as an argument; these give the provider additional
information about how to allocate the machine. For example, one can direct the
MAAS provider to acquire a particular node by specifying its hostname.
For more information on placement directives, see "juju help placement".

Examples:
   juju add-machine                      (starts a new machine)
   juju add-machine -n 2                 (starts 2 new machines)
   juju add-machine lxd                  (starts a new machine with an lxd container)
   juju add-machine lxd -n 2             (starts 2 new machines with an lxd container)
   juju add-machine lxd:4                (starts a new lxd container on machine 4)
   juju add-machine --constraints mem=8G (starts a machine with at least 8GB RAM)
   juju add-machine ssh:user@10.10.0.3   (manually provisions a machine with ssh)
   juju add-machine zone=us-east-1a      (start a machine in zone us-east-1a on AWS)
   juju add-machine maas2.name           (acquire machine maas2.name on MAAS)

See Also:
    juju remove-machine
`

func init() {
	containerTypes := make([]string, len(instance.ContainerTypes))
	for i, t := range instance.ContainerTypes {
		containerTypes[i] = string(t)
	}
	addMachineDoc = strings.Replace(
		addMachineDoc,
		"$CONTAINER_TYPES$",
		strings.Join(containerTypes, ", "),
		-1,
	)
}

// NewAddCommand returns a command that adds a machine to a model.
func NewAddCommand() cmd.Command {
	return modelcmd.Wrap(&addCommand{})
}

// addCommand starts a new machine and registers it in the model.
type addCommand struct {
	modelcmd.ModelCommandBase
	api               AddMachineAPI
	modelConfigAPI    ModelConfigAPI
	machineManagerAPI MachineManagerAPI
	// If specified, use this series, else use the model default-series
	Series string
	// If specified, these constraints are merged with those already in the model.
	Constraints constraints.Value
	// Placement is passed verbatim to the API, to be parsed and evaluated server-side.
	Placement *instance.Placement
	// NumMachines is the number of machines to add.
	NumMachines int
	// Disks describes disks that are to be attached to the machine.
	Disks []storage.Constraints
}

func (c *addCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-machine",
		Args:    "[<container>:machine | <container> | ssh:[user@]host | placement]",
		Purpose: "Start a new, empty machine and optionally a container, or add a container to a machine.",
		Doc:     addMachineDoc,
	}
}

func (c *addCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Series, "series", "", "The charm series")
	f.IntVar(&c.NumMachines, "n", 1, "The number of machines to add")
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "Additional machine constraints")
	f.Var(disksFlag{&c.Disks}, "disks", "Constraints for disks to attach to the machine")
}

func (c *addCommand) Init(args []string) error {
	if c.Constraints.Container != nil {
		return errors.Errorf("container constraint %q not allowed when adding a machine", *c.Constraints.Container)
	}
	placement, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.Placement, err = instance.ParsePlacement(placement)
	if err == instance.ErrPlacementScopeMissing {
		placement = "model-uuid" + ":" + placement
		c.Placement, err = instance.ParsePlacement(placement)
	}
	if err != nil {
		return err
	}
	if c.NumMachines > 1 && c.Placement != nil && c.Placement.Directive != "" {
		return errors.New("cannot use -n when specifying a placement directive")
	}
	return nil
}

type AddMachineAPI interface {
	AddMachines([]params.AddMachineParams) ([]params.AddMachinesResult, error)
	Close() error
	ForceDestroyMachines(machines ...string) error
	ModelUUID() (string, bool)
	ProvisioningScript(params.ProvisioningScriptParams) (script string, err error)
}

type ModelConfigAPI interface {
	ModelGet() (map[string]interface{}, error)
	Close() error
}

type MachineManagerAPI interface {
	AddMachines([]params.AddMachineParams) ([]params.AddMachinesResult, error)
	BestAPIVersion() int
	Close() error
}

var manualProvisioner = manual.ProvisionMachine

func (c *addCommand) getClientAPI() (AddMachineAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *addCommand) getModelConfigAPI() (ModelConfigAPI, error) {
	if c.modelConfigAPI != nil {
		return c.modelConfigAPI, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return modelconfig.NewClient(api), nil

}

func (c *addCommand) NewMachineManagerClient() (*machinemanager.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return machinemanager.NewClient(root), nil
}

func (c *addCommand) getMachineManagerAPI() (MachineManagerAPI, error) {
	if c.machineManagerAPI != nil {
		return c.machineManagerAPI, nil
	}
	return c.NewMachineManagerClient()
}

func (c *addCommand) Run(ctx *cmd.Context) error {
	client, err := c.getClientAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	var machineManager MachineManagerAPI
	if len(c.Disks) > 0 {
		machineManager, err = c.getMachineManagerAPI()
		if err != nil {
			return errors.Trace(err)
		}
		defer machineManager.Close()
		if machineManager.BestAPIVersion() < 1 {
			return errors.New("cannot add machines with disks: not supported by the API server")
		}
	}

	logger.Infof("load config")
	modelConfigClient, err := c.getModelConfigAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelConfigClient.Close()
	configAttrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return errors.Trace(err)
	}
	config, err := config.New(config.NoDefaults, configAttrs)
	if err != nil {
		return errors.Trace(err)
	}

	if c.Placement != nil && c.Placement.Scope == "ssh" {
		logger.Infof("manual provisioning")
		authKeys, err := common.ReadAuthorizedKeys(ctx, "")
		if err != nil {
			return errors.Annotate(err, "reading authorized-keys")
		}
		args := manual.ProvisionMachineArgs{
			Host:           c.Placement.Directive,
			Client:         client,
			Stdin:          ctx.Stdin,
			Stdout:         ctx.Stdout,
			Stderr:         ctx.Stderr,
			AuthorizedKeys: authKeys,
			UpdateBehavior: &params.UpdateBehavior{
				config.EnableOSRefreshUpdate(),
				config.EnableOSUpgrade(),
			},
		}
		machineId, err := manualProvisioner(args)
		if err == nil {
			ctx.Infof("created machine %v", machineId)
		}
		return err
	}

	logger.Infof("model provisioning")
	if c.Placement != nil && c.Placement.Scope == "model-uuid" {
		uuid, ok := client.ModelUUID()
		if !ok {
			return errors.New("API connection is controller-only (should never happen)")
		}
		c.Placement.Scope = uuid
	}

	if c.Placement != nil && c.Placement.Scope == instance.MachineScope {
		// It does not make sense to add-machine <id>.
		return errors.Errorf("machine-id cannot be specified when adding machines")
	}

	jobs := []multiwatcher.MachineJob{multiwatcher.JobHostUnits}

	machineParams := params.AddMachineParams{
		Placement:   c.Placement,
		Series:      c.Series,
		Constraints: c.Constraints,
		Jobs:        jobs,
		Disks:       c.Disks,
	}
	machines := make([]params.AddMachineParams, c.NumMachines)
	for i := 0; i < c.NumMachines; i++ {
		machines[i] = machineParams
	}

	var results []params.AddMachinesResult
	// If storage is specified, we attempt to use a new API on the service facade.
	if len(c.Disks) > 0 {
		results, err = machineManager.AddMachines(machines)
	} else {
		results, err = client.AddMachines(machines)
	}
	if params.IsCodeOperationBlocked(err) {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	if err != nil {
		return errors.Trace(err)
	}

	errs := []error{}
	for _, machineInfo := range results {
		if machineInfo.Error != nil {
			errs = append(errs, machineInfo.Error)
			continue
		}
		machineId := machineInfo.Machine

		if names.IsContainerMachine(machineId) {
			ctx.Infof("created container %v", machineId)
		} else {
			ctx.Infof("created machine %v", machineId)
		}
	}
	if len(errs) == 1 {
		fmt.Fprint(ctx.Stderr, "failed to create 1 machine\n")
		return errs[0]
	}
	if len(errs) > 1 {
		fmt.Fprintf(ctx.Stderr, "failed to create %d machines\n", len(errs))
		returnErr := []string{}
		for _, e := range errs {
			returnErr = append(returnErr, e.Error())
		}
		return errors.New(strings.Join(returnErr, ", "))
	}
	return nil
}
