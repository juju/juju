// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"launchpad.net/loggo"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/cloudinit/sshinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/rpc"
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

	machineParams, err := gatherMachineParams(args.Host)
	if err != nil {
		return "", err
	}
	arch := ""
	if machineParams.HardwareCharacteristics.Arch != nil {
		arch = *machineParams.HardwareCharacteristics.Arch
	}

	// Inform Juju that the machine exists.
	machineId, err = recordMachineInState(client, *machineParams)
	if rpc.IsNoSuchRequest(err) {
		logger.Infof("InjectMachines not supported by the API server, " +
			"falling back to 1.16 compatibility mode (direct DB access)")
		stateConn, err = juju.NewConnFromName(args.EnvName)
		if err == nil {
			machineId, err = recordMachineInState1dot16(stateConn, *machineParams)
		}
	}
	if err != nil {
		return "", err
	}

	var configParameters params.MachineConfig
	if stateConn == nil {
		configParameters, err = client.MachineConfig(machineId, machineParams.Series, arch)
	} else {
		request := params.MachineConfigParams{
			MachineId: machineId,
			Series: machineParams.Series,
			Arch: arch,
		}
		configParameters, err = statecmd.MachineConfig(stateConn.State, request)
	}
	if err != nil {
		return "", err
	}
	// Gather the information needed by the machine agent to run the provisioning script.
	mcfg, err := finishMachineConfig(configParameters, machineId, machineParams.Nonce, args.DataDir)
	if err != nil {
		return machineId, err
	}

	// Finally, provision the machine agent.
	err = provisionMachineAgent(args.Host, mcfg)
	if err != nil {
		return machineId, err
	}

	logger.Infof("Provisioned machine %v", machineId)
	return machineId, nil
}

func hostWithoutUser(host string) string {
	hostWithoutUser := host
	if at := strings.Index(hostWithoutUser, "@"); at != -1 {
		hostWithoutUser = hostWithoutUser[at+1:]
	}
	return hostWithoutUser
}

func recordMachineInState(
	client *api.Client, machineParams params.AddMachineParams) (machineId string, err error) {
	results, err := client.InjectMachines([]params.AddMachineParams{machineParams})
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
	stateParams := state.AddMachineParams{
		Series: machineParams.Series,
		Constraints: machineParams.Constraints, // not used
		Jobs: stateJobs,
		ParentId: machineParams.ParentId, //not used
		ContainerType: machineParams.ContainerType, // not used
		InstanceId: machineParams.InstanceId,
		HardwareCharacteristics: machineParams.HardwareCharacteristics,
		Nonce: machineParams.Nonce,
	}
	machine, err := stateConn.State.InjectMachine(&stateParams)
	if err != nil {
		return "", err
	}
	return machine.Id(), nil
}

func gatherMachineParams(host string) (*params.AddMachineParams, error) {

	// Generate a unique nonce for the machine.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	// First, gather the parameters needed to inject the existing host into state.
	sshHostWithoutUser := hostWithoutUser(host)
	if ip := net.ParseIP(sshHostWithoutUser); ip != nil {
		// Do a reverse-lookup on the IP. The IP may not have
		// a DNS entry, so just log a warning if this fails.
		names, err := net.LookupAddr(ip.String())
		if err != nil {
			logger.Infof("failed to resolve %v: %v", ip, err)
		} else {
			logger.Infof("resolved %v to %v", ip, names)
			sshHostWithoutUser = names[0]
		}
	}
	addrs, err := instance.HostAddresses(sshHostWithoutUser)
	if err != nil {
		return nil, err
	}
	logger.Infof("addresses for %v: %v", sshHostWithoutUser, addrs)

	provisioned, err := checkProvisioned(host)
	if err != nil {
		err = fmt.Errorf("error checking if provisioned: %v", err)
		return nil, err
	}
	if provisioned {
		return nil, ErrProvisioned
	}

	hc, series, err := DetectSeriesAndHardwareCharacteristics(host)
	if err != nil {
		err = fmt.Errorf("error detecting hardware characteristics: %v", err)
		return nil, err
	}

	// There will never be a corresponding "instance" that any provider
	// knows about. This is fine, and works well with the provisioner
	// task. The provisioner task will happily remove any and all dead
	// machines from state, but will ignore the associated instance ID
	// if it isn't one that the environment provider knows about.

	instanceId := instance.Id(manualInstancePrefix + hostWithoutUser(host))
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

func finishMachineConfig(configParameters params.MachineConfig, machineId, nonce, dataDir string) (*cloudinit.MachineConfig, error) {
	stateInfo := &state.Info{
		Addrs:    configParameters.StateAddrs,
		Password: configParameters.Password,
		Tag:      configParameters.Tag,
		CACert:   configParameters.CACert,
	}
	apiInfo := &api.Info{
		Addrs:    configParameters.APIAddrs,
		Password: configParameters.Password,
		Tag:      configParameters.Tag,
		CACert:   configParameters.CACert,
	}
	environConfig, err := config.New(config.NoDefaults, configParameters.EnvironAttrs)
	if err != nil {
		return nil, err
	}
	mcfg := environs.NewMachineConfig(machineId, nonce, stateInfo, apiInfo)
	if dataDir != "" {
		mcfg.DataDir = dataDir
	}
	mcfg.Tools = configParameters.Tools
	err = environs.FinishMachineConfig(mcfg, environConfig, constraints.Value{})
	if err != nil {
		return nil, err
	}
	return mcfg, nil
}

func provisionMachineAgent(host string, mcfg *cloudinit.MachineConfig) error {
	cloudcfg := coreCloudinit.New()
	if err := cloudinit.ConfigureJuju(mcfg, cloudcfg); err != nil {
		return err
	}
	// Explicitly disabling apt_upgrade so as not to trample
	// the target machine's existing configuration.
	cloudcfg.SetAptUpgrade(false)
	return sshinit.Configure(host, cloudcfg)
}
