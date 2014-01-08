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
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
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
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			client.DestroyMachines(machineId)
			machineId = ""
		}
		client.Close()
	}()

	// Create the "ubuntu" user and initialise passwordless sudo. We populate
	// the ubuntu user's authorized_keys file with the public keys in the current
	// user's ~/.ssh directory. The authenticationworker will later update the
	// ubuntu user's authorized_keys.
	user, host := splitUserHost(args.Host)
	authorizedKeys, err := config.ReadAuthorizedKeys("")
	if err := InitUbuntuUser(host, user, authorizedKeys, args.Stdin, args.Stdout); err != nil {
		return "", err
	}

	// Generate a unique nonce for the machine.
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", err
	}
	instanceId := instance.Id(manualInstancePrefix + host)
	nonce := fmt.Sprintf("%s:%s", instanceId, uuid.String())

	// Inform Juju that the machine exists.
	machineId, series, arch, err := recordMachineInState(client, host, nonce, instanceId)
	if err != nil {
		return "", err
	}

	// Gather the information needed by the machine agent to run the provisioning script.
	mcfg, err := createMachineConfig(client, machineId, series, arch, nonce, args.DataDir)
	if err != nil {
		return machineId, err
	}

	// Finally, provision the machine agent.
	err = provisionMachineAgent(host, mcfg, args.Stderr)
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
	client *api.Client, host, nonce string, instanceId instance.Id) (machineId, series, arch string, err error) {

	// First, gather the parameters needed to inject the existing host into state.
	if ip := net.ParseIP(host); ip != nil {
		// Do a reverse-lookup on the IP. The IP may not have
		// a DNS entry, so just log a warning if this fails.
		names, err := net.LookupAddr(ip.String())
		if err != nil {
			logger.Infof("failed to resolve %v: %v", ip, err)
		} else {
			logger.Infof("resolved %v to %v", ip, names)
			host = names[0]
		}
	}
	addrs, err := instance.HostAddresses(host)
	if err != nil {
		return "", "", "", err
	}
	logger.Infof("addresses for %v: %v", host, addrs)

	provisioned, err := checkProvisioned(host)
	if err != nil {
		err = fmt.Errorf("error checking if provisioned: %v", err)
		return "", "", "", err
	}
	if provisioned {
		return "", "", "", ErrProvisioned
	}

	hc, series, err := DetectSeriesAndHardwareCharacteristics(host)
	if err != nil {
		err = fmt.Errorf("error detecting hardware characteristics: %v", err)
		return "", "", "", err
	}

	// Inject a new machine into state.
	//
	// There will never be a corresponding "instance" that any provider
	// knows about. This is fine, and works well with the provisioner
	// task. The provisioner task will happily remove any and all dead
	// machines from state, but will ignore the associated instance ID
	// if it isn't one that the environment provider knows about.
	machineParams := params.AddMachineParams{
		Series:                  series,
		HardwareCharacteristics: hc,
		InstanceId:              instanceId,
		Nonce:                   nonce,
		Addrs:                   addrs,
		Jobs:                    []params.MachineJob{params.JobHostUnits},
	}
	results, err := client.AddMachines([]params.AddMachineParams{machineParams})
	if err != nil {
		return "", "", "", err
	}
	// Currently, only one machine is added, but in future there may be several added in one call.
	machineInfo := results[0]
	if machineInfo.Error != nil {
		return "", "", "", machineInfo.Error
	}
	return machineInfo.Machine, series, *hc.Arch, nil
}

func createMachineConfig(client *api.Client, machineId, series, arch, nonce, dataDir string) (*cloudinit.MachineConfig, error) {
	configParameters, err := client.MachineConfig(machineId, series, arch)
	if err != nil {
		return nil, err
	}
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

func provisionMachineAgent(host string, mcfg *cloudinit.MachineConfig, stderr io.Writer) error {
	cloudcfg := coreCloudinit.New()
	if err := cloudinit.ConfigureJuju(mcfg, cloudcfg); err != nil {
		return err
	}
	// Explicitly disabling apt_upgrade so as not to trample
	// the target machine's existing configuration.
	cloudcfg.SetAptUpgrade(false)
	return sshinit.Configure(sshinit.ConfigureParams{
		Host:   "ubuntu@" + host,
		Config: cloudcfg,
		Stderr: stderr,
	})
}
