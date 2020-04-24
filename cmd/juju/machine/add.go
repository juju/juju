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
	"github.com/juju/utils/winrm"

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

Examples:
   juju add-machine                      (starts a new machine)
   juju add-machine -n 2                 (starts 2 new machines)
   juju add-machine lxd                  (starts a new machine with an lxd container)
   juju add-machine lxd -n 2             (starts 2 new machines with an lxd container)
   juju add-machine lxd:4                (starts a new lxd container on machine 4)
   juju add-machine --constraints mem=8G (starts a machine with at least 8GB RAM)
   juju add-machine ssh:user@10.10.0.3   (manually provisions machine with ssh)
   juju add-machine winrm:user@10.10.0.3 (manually provisions machine with winrm)
   juju add-machine zone=us-east-1a      (start a machine in zone us-east-1a on AWS)
   juju add-machine maas2.name           (acquire machine maas2.name on MAAS)

See also:
    remove-machine
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
		Args:    "[<container>:machine | <container> | ssh:[user@]host | winrm:[user@]host | placement]",
		Purpose: "Start a new, empty machine and optionally a container, or add a container to a machine.",
		Doc:     addMachineDoc,
	})
}

func (c *addCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Series, "series", "", "The charm series")
	f.IntVar(&c.NumMachines, "n", 1, "The number of machines to add")
	f.StringVar(&c.ConstraintsStr, "constraints", "", "Additional machine constraints")
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
