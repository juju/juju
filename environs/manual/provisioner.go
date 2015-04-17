// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/sshinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
)

const manualInstancePrefix = "manual:"

var logger = loggo.GetLogger("juju.environs.manual")

// ProvisioningClientAPI defines the methods that are needed for the manual
// provisioning of machines.  An interface is used here to decouple the API
// consumer from the actual API implementation type.
type ProvisioningClientAPI interface {
	AddMachines([]params.AddMachineParams) ([]params.AddMachinesResult, error)
	ForceDestroyMachines(machines ...string) error
	ProvisioningScript(params.ProvisioningScriptParams) (script string, err error)
}

type ProvisionMachineArgs struct {
	// Host is the SSH host: [user@]host
	Host string

	// DataDir is the root directory for juju data.
	// If left blank, the default location "/var/lib/juju" will be used.
	DataDir string

	// Client provides the API needed to provision the machines.
	Client ProvisioningClientAPI

	// Stdin is required to respond to sudo prompts,
	// and must be a terminal (except in tests)
	Stdin io.Reader

	// Stdout is required to present sudo prompts to the user.
	Stdout io.Writer

	// Stderr is required to present machine provisioning progress to the user.
	Stderr io.Writer

	*params.UpdateBehavior
}

// ErrProvisioned is returned by ProvisionMachine if the target
// machine has an existing machine agent.
var ErrProvisioned = errors.New("machine is already provisioned")

// ProvisionMachine provisions a machine agent to an existing host, via
// an SSH connection to the specified host. The host may optionally be preceded
// with a login username, as in [user@]host.
//
// On successful completion, this function will return the id of the state.Machine
// that was entered into state.
func ProvisionMachine(args ProvisionMachineArgs) (machineId string, err error) {
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			if cleanupErr := args.Client.ForceDestroyMachines(machineId); cleanupErr != nil {
				logger.Warningf("error cleaning up machine: %s", cleanupErr)
			}
			machineId = ""
		}
	}()

	// Create the "ubuntu" user and initialise passwordless sudo. We populate
	// the ubuntu user's authorized_keys file with the public keys in the current
	// user's ~/.ssh directory. The authenticationworker will later update the
	// ubuntu user's authorized_keys.
	user, hostname := splitUserHost(args.Host)
	authorizedKeys, err := config.ReadAuthorizedKeys("")
	if err := InitUbuntuUser(hostname, user, authorizedKeys, args.Stdin, args.Stdout); err != nil {
		return "", err
	}

	machineParams, err := gatherMachineParams(hostname)
	if err != nil {
		return "", err
	}

	// Inform Juju that the machine exists.
	machineId, err = recordMachineInState(args.Client, *machineParams)
	if err != nil {
		return "", err
	}

	provisioningScript, err := args.Client.ProvisioningScript(params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     machineParams.Nonce,
		DisablePackageCommands: !args.EnableOSRefreshUpdate && !args.EnableOSUpgrade,
	})

	if err != nil {
		logger.Errorf("cannot obtain provisioning script")
		return "", err
	}

	// Finally, provision the machine agent.
	err = runProvisionScript(provisioningScript, hostname, args.Stderr)
	if err != nil {
		return machineId, err
	}

	logger.Infof("Provisioned machine %v", machineId)
	return machineId, nil
}

func splitUserHost(host string) (string, string) {
	if at := strings.Index(host, "@"); at != -1 {
		return host[:at], host[at+1:]
	}
	return "", host
}

func recordMachineInState(client ProvisioningClientAPI, machineParams params.AddMachineParams) (machineId string, err error) {
	results, err := client.AddMachines([]params.AddMachineParams{machineParams})
	if err != nil {
		return "", err
	}
	// Currently, only one machine is added, but in future there may be several added in one call.
	machineInfo := results[0]
	if machineInfo.Error != nil {
		return "", machineInfo.Error
	}
	return machineInfo.Machine, nil
}

// gatherMachineParams collects all the information we know about the machine
// we are about to provision. It will SSH into that machine as the ubuntu user.
// The hostname supplied should not include a username.
// If we can, we will reverse lookup the hostname by its IP address, and use
// the DNS resolved name, rather than the name that was supplied
func gatherMachineParams(hostname string) (*params.AddMachineParams, error) {

	// Generate a unique nonce for the machine.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}

	var addrs []network.Address
	if addr, err := HostAddress(hostname); err != nil {
		logger.Warningf("failed to compute public address for %q: %v", hostname, err)
	} else {
		addrs = append(addrs, addr)
	}

	provisioned, err := checkProvisioned(hostname)
	if err != nil {
		err = fmt.Errorf("error checking if provisioned: %v", err)
		return nil, err
	}
	if provisioned {
		return nil, ErrProvisioned
	}

	hc, series, err := DetectSeriesAndHardwareCharacteristics(hostname)
	if err != nil {
		err = fmt.Errorf("error detecting hardware characteristics: %v", err)
		return nil, err
	}

	// There will never be a corresponding "instance" that any provider
	// knows about. This is fine, and works well with the provisioner
	// task. The provisioner task will happily remove any and all dead
	// machines from state, but will ignore the associated instance ID
	// if it isn't one that the environment provider knows about.
	// Also, manually provisioned machines don't have the JobManageNetworking.
	// This ensures that the networker is running in non-intrusive mode
	// and never touches the network configuration files.
	// No JobManageNetworking here due to manual provisioning.

	instanceId := instance.Id(manualInstancePrefix + hostname)
	nonce := fmt.Sprintf("%s:%s", instanceId, uuid.String())
	machineParams := &params.AddMachineParams{
		Series:                  series,
		HardwareCharacteristics: hc,
		InstanceId:              instanceId,
		Nonce:                   nonce,
		Addrs:                   params.FromNetworkAddresses(addrs),
		Jobs:                    []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
	}
	return machineParams, nil
}

var provisionMachineAgent = func(host string, icfg *instancecfg.InstanceConfig, progressWriter io.Writer) error {
	script, err := ProvisioningScript(icfg)
	if err != nil {
		return err
	}
	return runProvisionScript(script, host, progressWriter)
}

// ProvisioningScript generates a bash script that can be
// executed on a remote host to carry out the cloud-init
// configuration.
func ProvisioningScript(icfg *instancecfg.InstanceConfig) (string, error) {
	cloudcfg, err := cloudinit.New(icfg.Series)
	if err != nil {
		return "", errors.Annotate(err, "error generating cloud-config")
	}
	cloudcfg.SetSystemUpdate(icfg.EnableOSRefreshUpdate)
	cloudcfg.SetSystemUpgrade(icfg.EnableOSUpgrade)

	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
	if err != nil {
		return "", errors.Annotate(err, "error generating cloud-config")
	}
	if err := udata.ConfigureJuju(); err != nil {
		return "", errors.Annotate(err, "error generating cloud-config")
	}

	configScript, err := cloudcfg.RenderScript()
	if err != nil {
		return "", errors.Annotate(err, "error converting cloud-config to script")
	}

	var buf bytes.Buffer
	// Always remove the cloud-init-output.log file first, if it exists.
	fmt.Fprintf(&buf, "rm -f %s\n", utils.ShQuote(icfg.CloudInitOutputLog))
	// If something goes wrong, dump cloud-init-output.log to stderr.
	buf.WriteString(shell.DumpFileOnErrorScript(icfg.CloudInitOutputLog))
	buf.WriteString(configScript)
	return buf.String(), nil
}

func runProvisionScript(script, host string, progressWriter io.Writer) error {
	params := sshinit.ConfigureParams{
		Host:           "ubuntu@" + host,
		ProgressWriter: progressWriter,
	}
	return sshinit.RunConfigureScript(script, params)
}
