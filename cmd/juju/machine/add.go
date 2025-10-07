// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

var addMachineDoc = `
Add a new machine to the model.

The command operates in three modes, depending on the options provided:

  - provision a new machine from the cloud (default, see "Provisioning
    a new machine")
  - create an operating system container (see "Container creation")
  - connect to a live computer and allocate it as a machine (see "Manual
    provisioning")

The ` + "`add-machine` " + `command is unavailable in Kubernetes clouds. Provisioning
a new machine is unavailable on the manual cloud provider.

Once the` + "` add-machine` " + `command has finished, the machine's ID can be
used as a placement directive for deploying applications. Machine IDs
are also accessible via ` + "`juju status` " + `and ` + "`juju machines`" + `.


### Provisioning a new machine

When add-machine is called without arguments, Juju provisions a new
machine instance from the current cloud. The machine's specifications,
including whether the machine is virtual or physical, depend on the cloud.

To control which instance type is provisioned, use the ` + "`--constraints` " + ` and
` + "`--base` " + `options. ` + "`--base`" + ` can be specified using the OS name and the version of
the OS, separated by ` + "`@`" + `. For example, ` + "`--base ubuntu@22.04`" + `.

To add storage volumes to the instance, provide a whitespace-delimited
list of storage constraints to the ` + "`--disks` " + `option.

Add placement directives as an argument to ` + "`--to` " + `to give Juju additional information
about how to allocate the machine in the cloud. For example, one can direct
the MAAS provider to acquire a particular node by specifying its hostname.


### Manual provisioning

Call ` + "`add-machine` " + ` with the address of a network-accessible computer to
allocate that machine to the model.

Manual provisioning is the process of installing Juju on an existing machine
and bringing it under Juju's management. The Juju controller must be able to
access the new machine over the network.


### Container creation

If a operating system container type is specified (e.g., ` + "`lxd`" + ` or ` + "`kvm`" + `),
then ` + "`add-machine` " + `will allocate a container of that type on a new machine
instance. Both the new instance, and the new container will be available
as machines in the model.

It is also possible to add containers to existing machines using the format
` + "`<container-type>:<machine-id>`" + `. Constraints cannot be combined this mode.

`

const addMachineExamples = `
Start a new machine by requesting one from the cloud provider:

	juju add-machine

Start 2 new machines:

	juju add-machine -n 2

Start a LXD container on a new machine instance and add both as machines:

	juju add-machine lxd

Start two machine instances, each hosting a LXD container, then add all four as machines:

	juju add-machine lxd -n 2

Create a container on machine 4 and add it as a machine:

	juju add-machine lxd:4

Start a new machine and require that it has 8GB RAM:

	juju add-machine --constraints mem=8G

Start a new machine within the "us-east-1a" availability zone:

	juju add-machine --constraints zones=us-east-1a

Start a new machine with at least 4 CPU cores and 16GB RAM, and request three storage volumes to be attached to it. Two are large capacity (1TB) HDD and one is a lower capacity (100GB) SSD. Note: 'ebs' and 'ebs-ssd' are storage pools specific to AWS.

	juju add-machine --constraints="cores=4 mem=16G" --disks="ebs,1T,2 ebs-ssd,100G,1"

Allocate a machine to the model via SSH:

	juju add-machine ssh:user@10.10.0.3

Allocate a machine specifying the private key to use during the connection:

	juju add-machine ssh:user@10.10.0.3 --private-key /tmp/id_rsa

Allocate a machine specifying a public key to set in the list of authorized keys in the machine:

	juju add-machine ssh:user@10.10.0.3 --public-key /tmp/id_rsa.pub

Allocate a machine specifying a public key to set in the list of authorized keys and the private key to used during the connection:

	juju add-machine ssh:user@10.10.0.3 --public-key /tmp/id_rsa.pub --private-key /tmp/id_rsa

Allocate a machine to the model. Note: specific to MAAS.

	juju add-machine host.internal
`

// NewAddCommand returns a command that adds a machine to a model.
func NewAddCommand() cmd.Command {
	return modelcmd.Wrap(&addCommand{})
}

// addCommand starts a new machine and registers it in the model.
type addCommand struct {
	baseMachinesCommand
	modelConfigAPI    ModelConfigAPI
	machineManagerAPI MachineManagerAPI
	// Series defines the series the machine should use instead of the
	// default-series. DEPRECATED use --base
	Series string
	// Base defines the series the machine should use instead of the
	// default-base.
	Base string
	// If specified, these constraints are merged with those already in the model.
	Constraints constraints.Value
	// If specified, these constraints are merged with those already in the model.
	ConstraintsStr common.ConstraintsFlag
	// Placement is passed verbatim to the API, to be parsed and evaluated server-side.
	Placement *instance.Placement
	// NumMachines is the number of machines to add.
	NumMachines int
	// Disks describes disks that are to be attached to the machine.
	Disks []storage.Constraints
	// PrivateKey is the path for a file containing the private key required
	// by the server
	PrivateKey string
	// PublicKey is the path for a file containing a public key required
	// by the server
	PublicKey string
}

func (c *addCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-machine",
		Args:     "[<container-type>[:<machine-id>] | ssh:[<user>@]<host> | <placement>] | <private-key> | <public-key>",
		Purpose:  "Provision a new machine or assign one to the model.",
		Doc:      addMachineDoc,
		Examples: addMachineExamples,
		SeeAlso: []string{
			"remove-machine",
			"model-constraints",
			"set-model-constraints",
		},
	})
}

func (c *addCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Series, "series", "", "Specify the operating system series to install on the new machine(s). DEPRECATED use --base")
	f.StringVar(&c.Base, "base", "", "Specify the operating system base to install on the new machine(s)")
	f.IntVar(&c.NumMachines, "n", 1, "Specify the number of machines to add")
	f.Var(&c.ConstraintsStr, "constraints", "Specify the machine constraints to overwrite those available from `juju model-constraints` and provider's defaults")
	f.Var(disksFlag{&c.Disks}, "disks", "Specify the storage constraints for disks to attach to the machine(s)")
	f.StringVar(&c.PrivateKey, "private-key", "", "Specify the path to the private key to use during the connection")
	f.StringVar(&c.PublicKey, "public-key", "", "Specify the path to the public key to add to the remote authorized keys")
}

func (c *addCommand) Init(args []string) error {
	if c.Base != "" && c.Series != "" {
		return errors.New("--series and --base cannot be specified together")
	}
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

type ModelConfigAPI interface {
	ModelGet() (map[string]interface{}, error)
	Close() error
}

type MachineManagerAPI interface {
	AddMachines([]params.AddMachineParams) ([]params.AddMachinesResult, error)
	DestroyMachinesWithParams(force, keep, dryRun bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error)
	ModelUUID() (string, bool)
	ProvisioningScript(params.ProvisioningScriptParams) (script string, err error)
	Close() error
}

// splitUserHost given a host string of example user@192.168.122.122
// it will return user and 192.168.122.122
func splitUserHost(host string) (string, string) {
	if at := strings.Index(host, "@"); at != -1 {
		return host[:at], host[at+1:]
	}
	return "", host
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

func (c *addCommand) newMachineManagerClient() (*machinemanager.Client, error) {
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
	return c.newMachineManagerClient()
}

func (c *addCommand) Run(ctx *cmd.Context) error {
	var (
		base corebase.Base
		err  error
	)
	// Note: we validated that both series and base cannot be specified in
	// Init(), so it's safe to assume that only one of them is set here.
	if c.Series != "" {
		if c.Series == "kubernetes" {
			return fmt.Errorf(`command "add-machine" does not support container models`)
		} else {
			ctx.Warningf("series flag is deprecated, use --base instead")
			if base, err = corebase.GetBaseFromSeries(c.Series); err != nil {
				return errors.Annotatef(err, "attempting to convert %q to a base", c.Series)
			}
		}
		c.Base = base.String()
		c.Series = ""
	}
	if c.Base != "" {
		if base, err = corebase.ParseBaseFromString(c.Base); err != nil {
			return errors.Trace(err)
		}
	}

	c.Constraints, err = common.ParseConstraints(ctx, strings.Join(c.ConstraintsStr, " "))
	if err != nil {
		return err
	}
	machineManager, err := c.getMachineManagerAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer machineManager.Close()

	logger.Infof("load config")
	modelConfigClient, err := c.getModelConfigAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelConfigClient.Close()
	configAttrs, err := modelConfigClient.ModelGet()
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "add a machine to this model")
		}
		return errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, configAttrs)
	if err != nil {
		return errors.Trace(err)
	}

	if c.Placement != nil {
		err := c.tryManualProvision(machineManager, cfg, ctx)
		if err != errNonManualScope {
			return err
		}
	}

	logger.Infof("model provisioning")
	if c.Placement != nil && c.Placement.Scope == "model-uuid" {
		uuid, ok := machineManager.ModelUUID()
		if !ok {
			return errors.New("API connection is controller-only (should never happen)")
		}
		c.Placement.Scope = uuid
	}

	if c.Placement != nil && c.Placement.Scope == instance.MachineScope {
		// It does not make sense to add-machine <id>.
		return errors.Errorf("machine-id cannot be specified when adding machines")
	}

	jobs := []model.MachineJob{model.JobHostUnits}

	var paramsBase *params.Base
	if !base.Empty() {
		paramsBase = &params.Base{
			Name:    base.OS,
			Channel: base.Channel.String(),
		}
	}

	machineParams := params.AddMachineParams{
		Placement:   c.Placement,
		Base:        paramsBase,
		Constraints: c.Constraints,
		Jobs:        jobs,
		Disks:       c.Disks,
	}
	machines := make([]params.AddMachineParams, c.NumMachines)
	for i := 0; i < c.NumMachines; i++ {
		machines[i] = machineParams
	}

	results, err := machineManager.AddMachines(machines)
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

var (
	sshProvisioner    = sshprovisioner.ProvisionMachine
	errNonManualScope = errors.New("non-manual scope")
	sshScope          = "ssh"
)

func (c *addCommand) tryManualProvision(client manual.ProvisioningClientAPI, config *config.Config, ctx *cmd.Context) error {

	var provisionMachine manual.ProvisionMachineFunc
	switch c.Placement.Scope {
	case sshScope:
		provisionMachine = sshProvisioner
	default:
		return errNonManualScope
	}

	authKeys, err := common.ReadAuthorizedKeys(ctx, c.PublicKey)
	if err != nil {
		return errors.Annotatef(err, "cannot reading authorized-keys")
	}

	user, host := splitUserHost(c.Placement.Directive)
	args := manual.ProvisionMachineArgs{
		Host:           host,
		User:           user,
		Client:         client,
		Stdin:          ctx.Stdin,
		Stdout:         ctx.Stdout,
		Stderr:         ctx.Stderr,
		AuthorizedKeys: authKeys,
		PrivateKey:     c.PrivateKey,
		UpdateBehavior: &params.UpdateBehavior{
			EnableOSRefreshUpdate: config.EnableOSRefreshUpdate(),
			EnableOSUpgrade:       config.EnableOSUpgrade(),
		},
	}

	machineId, err := provisionMachine(args)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("created machine %v", machineId)
	return nil
}
