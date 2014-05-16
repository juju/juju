// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/juju/loggo"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/cloudinit/sshinit"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/shell"
)

const manualInstancePrefix = "manual:"

var logger = loggo.GetLogger("juju.environs.manual")

type ProvisionMachineArgs struct {
	// Host is the SSH host: [user@]host
	Host string

	// DataDir is the root directory for juju data.
	// If left blank, the default location "/var/lib/juju" will be used.
	DataDir string

	// EnvName is the name of the environment for which the machine will be provisioned.
	EnvName string

	// Tools to install on the machine. If nil, tools will be automatically
	// chosen using environs/tools FindInstanceTools.
	Tools *tools.Tools

	// Stdin is required to respond to sudo prompts,
	// and must be a terminal (except in tests)
	Stdin io.Reader

	// Stdout is required to present sudo prompts to the user.
	Stdout io.Writer

	// Stderr is required to present machine provisioning progress to the user.
	Stderr io.Writer
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
	client, err := juju.NewAPIClientFromName(args.EnvName)
	if err != nil {
		return "", err
	}
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			if cleanupErr := client.DestroyMachines(machineId); cleanupErr != nil {
				logger.Warningf("error cleaning up machine: %s", cleanupErr)
			}
			machineId = ""
		}
		client.Close()
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
	machineId, err = recordMachineInState(client, *machineParams)
	if err != nil {
		return "", err
	}

	provisioningScript, err := client.ProvisioningScript(params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     machineParams.Nonce,
	})
	if err != nil {
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

func recordMachineInState(
	client *api.Client, machineParams params.AddMachineParams) (machineId string, err error) {
	// Note: we explicitly use AddMachines1dot18 rather than AddMachines to preserve
	// backwards compatibility; we do not require any of the new features of AddMachines
	// here.
	results, err := client.AddMachines1dot18([]params.AddMachineParams{machineParams})
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

// convertToStateJobs takes a slice of params.MachineJob and makes them a slice of state.MachineJob
func convertToStateJobs(jobs []params.MachineJob) ([]state.MachineJob, error) {
	outJobs := make([]state.MachineJob, len(jobs))
	var err error
	for j, job := range jobs {
		if outJobs[j], err = state.MachineJobFromParams(job); err != nil {
			return nil, err
		}
	}
	return outJobs, nil
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

	var addrs []instance.Address
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

	instanceId := instance.Id(manualInstancePrefix + hostname)
	nonce := fmt.Sprintf("%s:%s", instanceId, uuid.String())
	machineParams := &params.AddMachineParams{
		Series:                  series,
		HardwareCharacteristics: hc,
		InstanceId:              instanceId,
		Nonce:                   nonce,
		Addrs:                   addrs,
		Jobs:                    []params.MachineJob{params.JobHostUnits},
	}
	return machineParams, nil
}

var provisionMachineAgent = func(host string, mcfg *cloudinit.MachineConfig, progressWriter io.Writer) error {
	script, err := ProvisioningScript(mcfg)
	if err != nil {
		return err
	}
	return runProvisionScript(script, host, progressWriter)
}

// ProvisioningScript generates a bash script that can be
// executed on a remote host to carry out the cloud-init
// configuration.
func ProvisioningScript(mcfg *cloudinit.MachineConfig) (string, error) {
	cloudcfg := coreCloudinit.New()
	if err := cloudinit.ConfigureJuju(mcfg, cloudcfg); err != nil {
		return "", err
	}
	// Explicitly disabling apt_upgrade so as not to trample
	// the target machine's existing configuration.
	cloudcfg.SetAptUpgrade(false)
	configScript, err := sshinit.ConfigureScript(cloudcfg)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	// Always remove the cloud-init-output.log file first, if it exists.
	fmt.Fprintf(&buf, "rm -f %s\n", utils.ShQuote(mcfg.CloudInitOutputLog))
	// If something goes wrong, dump cloud-init-output.log to stderr.
	buf.WriteString(shell.DumpFileOnErrorScript(mcfg.CloudInitOutputLog))
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
