// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"launchpad.net/loggo"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/cloudinit/sshinit"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
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
	// Used for fallback to 1.16 code
	var stateConn *juju.Conn
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			// If we have stateConn, then we are in 1.16
			// compatibility mode and we should issue
			// DestroyMachines directly on the state, rather than
			// via API (because DestroyMachine *also* didn't exist
			// in 1.16, though it will be in 1.16.5).
			// TODO: When this compatibility code is removed, we
			// should remove the method in state as well (as long
			// as destroy-machine also no longer needs it.)
			var cleanupErr error
			if stateConn != nil {
				cleanupErr = statecmd.DestroyMachines1dot16(stateConn.State, machineId)
			} else {
				cleanupErr = client.DestroyMachines(machineId)
			}
			if cleanupErr != nil {
				logger.Warningf("error cleaning up machine: %s", cleanupErr)
			}
			machineId = ""
		}
		if stateConn != nil {
			stateConn.Close()
			stateConn = nil
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
	if params.IsCodeNotImplemented(err) {
		logger.Infof("AddMachines not supported by the API server, " +
			"falling back to 1.16 compatibility mode (direct DB access)")
		stateConn, err = juju.NewConnFromName(args.EnvName)
		if err == nil {
			machineId, err = recordMachineInState1dot16(stateConn, *machineParams)
		}
	}
	if err != nil {
		return "", err
	}

	var provisioningScript string
	if stateConn == nil {
		provisioningScript, err = client.ProvisioningScript(machineId, machineParams.Nonce)
		if err != nil {
			return "", err
		}
	} else {
		mcfg, err := statecmd.MachineConfig(stateConn.State, machineId, machineParams.Nonce, args.DataDir)
		if err == nil {
			provisioningScript, err = generateProvisioningScript(mcfg)
		}
		if err != nil {
			return "", err
		}
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

func recordMachineInState1dot16(
	stateConn *juju.Conn, machineParams params.AddMachineParams) (machineId string, err error) {
	stateJobs, err := convertToStateJobs(machineParams.Jobs)
	if err != nil {
		return "", err
	}
	//if p.Series == "" {
	//	p.Series = defaultSeries
	//}
	template := state.MachineTemplate{
		Series:      machineParams.Series,
		Constraints: machineParams.Constraints,
		InstanceId:  machineParams.InstanceId,
		Jobs:        stateJobs,
		Nonce:       machineParams.Nonce,
		HardwareCharacteristics: machineParams.HardwareCharacteristics,
		Addresses:               machineParams.Addrs,
	}
	machine, err := stateConn.State.AddOneMachine(template)
	if err != nil {
		return "", err
	}
	return machine.Id(), nil
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
	// First, gather the parameters needed to inject the existing host into state.
	if ip := net.ParseIP(hostname); ip != nil {
		// Do a reverse-lookup on the IP. The IP may not have
		// a DNS entry, so just log a warning if this fails.
		names, err := net.LookupAddr(ip.String())
		if err != nil {
			logger.Infof("failed to resolve %v: %v", ip, err)
		} else {
			logger.Infof("resolved %v to %v", ip, names)
			hostname = names[0]
			// TODO: jam 2014-01-09 https://bugs.launchpad.net/bugs/1267387
			// We change what 'hostname' we are using here (rather
			// than an IP address we use the DNS name). I'm not
			// sure why that is better, but if we are changing the
			// host, we should probably be returning the hostname
			// to the parent function.
			// Also, we don't seem to try and compare if 'ip' is in
			// the list of addrs returned from
			// instance.HostAddresses in case you might get
			// multiple and one of them is what you are supposed to
			// be using.
		}
	}
	addrs, err := instance.HostAddresses(hostname)
	if err != nil {
		return nil, err
	}
	logger.Infof("addresses for %v: %v", hostname, addrs)

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

func provisionMachineAgent(host string, mcfg *cloudinit.MachineConfig, progressWriter io.Writer) error {
	script, err := generateProvisioningScript(mcfg)
	if err != nil {
		return err
	}
	return runProvisionScript(script, host, progressWriter)
}

func generateProvisioningScript(mcfg *cloudinit.MachineConfig) (string, error) {
	cloudcfg := coreCloudinit.New()
	if err := cloudinit.ConfigureJuju(mcfg, cloudcfg); err != nil {
		return "", err
	}
	// Explicitly disabling apt_upgrade so as not to trample
	// the target machine's existing configuration.
	cloudcfg.SetAptUpgrade(false)
	return sshinit.ConfigureScript(cloudcfg)
}

func runProvisionScript(script, host string, progressWriter io.Writer) error {
	params := sshinit.ConfigureParams{
		Host:           "ubuntu@" + host,
		ProgressWriter: progressWriter,
	}
	return sshinit.RunConfigureScript(script, params)
}
