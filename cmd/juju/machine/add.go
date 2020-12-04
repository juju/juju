// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v2/winrm"

	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/environs/manual/winrmprovisioner"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/storage"
)

var addMachineDoc = `
Add a new machine to the model. The command operates in three modes,
depending on the options provided:

  - provision a new machine from the cloud (default, see "Provisioning
    a new machine")
  - create an operating system container (see "Container creation")
  - connect to a live computer and allocate it as a machine (see "Manual 
    provisioning")

The add-machine command is unavailable in k8s clouds. Provisioning
a new machine is unavailable on the manual cloud provider. 

Once the add-machine command has finished, the machine's ID can be 
used as a placement directive for deploying applications. Machine IDs 
are also accessible via 'juju status' and 'juju machines'.


Provisioning a new machine

When add-machine is called without arguments, Juju provisions a new 
machine instance from the current cloud. The machine's specifications, 
including whether the machine is virtual or physical depends on the cloud.

To control which instance type is provisioned, use the --constraints and 
--series options.

To add storage volumes to the instance, provide a whitespace-delimited
list of storage constraints to the --disks option. 

Add "placement directives" as an argument give Juju additional information 
about how to allocate the machine in the cloud. For example, one can direct 
the MAAS provider to acquire a particular node by specifying its hostname.


Manual provisioning

Call add-machine with the address of a network-accessible computer to 
allocate that machine to the model. You should connect 

Manual provisioning is the process of installing Juju on an existing machine
and bringing it under Juju's management. The Juju controller must be able to
access the new machine over the network.


Container creation

If a operating system container type is specified (e.g. "lxd" or "kvm"), 
then add-machine will allocate a container of that type on a new machine 
instance. Both the new instance, and the new container will be available 
as machines in the model.

It is also possible to add containers to existing machines using the format
<container-type>:<machine-id>. Constraints cannot be combined this mode.


Examples:

	# Start a new machine by requesting one from the cloud provider.
	juju add-machine
	
	# Start 2 new machines.
	juju add-machine -n 2

	# Start a LXD container on a new machine instance and add both as machines. 
	juju add-machine lxd

	# Start two machine instances, each hosting a LXD container, then add all
	# four as machines. 
	juju add-machine lxd -n 2

	# Create a container on machine 4 and add it as a machine.
	juju add-machine lxd:4

	# Start a new machine and require that it has 8GB RAM
	juju add-machine --constraints mem=8G

	# Start a new machine within the "us-east-1a" availability zone.
	juju add-machine --constraints zone=us-east-1a

	# Start a new machine with at least 4 CPU cores and 16GB RAM, and request
	# three storage volumes to be attached to it. Two are large capacity (1TB)
	# HDD and one is a lower capacity (100GB) SSD. Note: "ebs" and "ebs-ssd" 
	# are storage pools specific to AWS. 
	juju add-machine --constraints="cores=4 mem=16G" --disks="ebs,1T,2 ebs-ssd,100G,1"

	# Allocate a machine to the model via SSH
	juju add-machine ssh:user@10.10.0.3

	# Allocate a machine to the model via WinRM
	juju add-machine winrm:user@10.10.0.3

	# Allocate a machine to the model. Note: specific to MAAS.
	juju add-machine host.internal


Further reading:
	https://juju.is/docs/reference/commands/add-machine
	https://juju.is/docs/reference/constraints

See also:
	remove-machine
	get-model-constraints
	set-model-constraints
`

// NewAddCommand returns a command that adds a machine to a model.
func NewAddCommand() cmd.Command {
	return modelcmd.Wrap(&addCommand{})
}

// addCommand starts a new machine and registers it in the model.
type addCommand struct {
	baseMachinesCommand
	api               AddMachineAPI
	modelConfigAPI    ModelConfigAPI
	machineManagerAPI MachineManagerAPI
	// If specified, use this series, else use the model default-series
	Series string
	// If specified, these constraints are merged with those already in the model.
	Constraints constraints.Value
	// If specified, these constraints are merged with those already in the model.
	ConstraintsStr string
	// Placement is passed verbatim to the API, to be parsed and evaluated server-side.
	Placement *instance.Placement
	// NumMachines is the number of machines to add.
	NumMachines int
	// Disks describes disks that are to be attached to the machine.
	Disks []storage.Constraints
}

func (c *addCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add-machine",
		Args:    "[<container-type>[:<machine-id>] | (ssh|winrm):[<user>@]<host> | <placement>]",
		Purpose: "Provision a new machine or assign one to the model.",
		Doc:     addMachineDoc,
	})
}

func (c *addCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Series, "series", "", "The operating system series to install on the new machine(s)")
	f.IntVar(&c.NumMachines, "n", 1, "The number of machines to add")
	f.StringVar(&c.ConstraintsStr, "constraints", "", "Machine constraints that overwrite those available from 'juju get-model-constraints' and provider's defaults")
	f.Var(disksFlag{&c.Disks}, "disks", "Storage constraints for disks to attach to the machine(s)")
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
	DestroyMachinesWithParams(force, keep bool, machines ...string) error
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

// splitUserHost given a host string of example user@192.168.122.122
// it will return user and 192.168.122.122
func splitUserHost(host string) (string, string) {
	if at := strings.Index(host, "@"); at != -1 {
		return host[:at], host[at+1:]
	}
	return "", host
}

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
	var err error
	c.Constraints, err = common.ParseConstraints(ctx, c.ConstraintsStr)
	if err != nil {
		return err
	}
	client, err := c.getClientAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	machineManager, err := c.getMachineManagerAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer machineManager.Close()

	if len(c.Disks) > 0 && machineManager.BestAPIVersion() < 1 {
		return errors.New("cannot add machines with disks: not supported by the API server")
	}

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
		err := c.tryManualProvision(client, cfg, ctx)
		if err != errNonManualScope {
			return err
		}
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

	jobs := []model.MachineJob{model.JobHostUnits}

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
	winrmProvisioner  = winrmprovisioner.ProvisionMachine
	errNonManualScope = errors.New("non-manual scope")
	sshScope          = "ssh"
	winrmScope        = "winrm"
)

func (c *addCommand) tryManualProvision(client AddMachineAPI, config *config.Config, ctx *cmd.Context) error {

	var provisionMachine manual.ProvisionMachineFunc
	switch c.Placement.Scope {
	case sshScope:
		provisionMachine = sshProvisioner
	case winrmScope:
		provisionMachine = c.provisionWinRM
	default:
		return errNonManualScope
	}

	authKeys, err := common.ReadAuthorizedKeys(ctx, "")
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

func (c *addCommand) provisionWinRM(args manual.ProvisionMachineArgs) (string, error) {
	base := osenv.JujuXDGDataHomePath("x509")
	keyPath := filepath.Join(base, "winrmkey.pem")
	certPath := filepath.Join(base, "winrmcert.crt")
	cert := winrm.NewX509()
	if err := cert.LoadClientCert(keyPath, certPath); err != nil {
		return "", errors.Annotatef(err, "connot load/create x509 client certs for winrm connection")
	}
	if err := cert.LoadCACert(filepath.Join(base, "winrmcacert.crt")); err != nil {
		logger.Infof("cannot not find any CA cert to load")
	}

	cfg := winrm.ClientConfig{
		User:    args.User,
		Host:    args.Host,
		Key:     cert.ClientKey(),
		Cert:    cert.ClientCert(),
		Timeout: 25 * time.Second,
		Secure:  true,
	}

	caCert := cert.CACert()
	if caCert == nil {
		logger.Infof("Skipping winrm CA validation")
		cfg.Insecure = true
	} else {
		cfg.CACert = caCert
	}

	client, err := winrm.NewClient(cfg)
	if err != nil {
		return "", errors.Annotatef(err, "cannot create WinRM client connection")
	}
	args.WinRM = manual.WinRMArgs{
		Keys:   cert,
		Client: client,
	}
	return winrmProvisioner(args)
}
