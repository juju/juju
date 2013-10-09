// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker/provisioner"
)

const manualInstancePrefix = "manual:"

var logger = loggo.GetLogger("juju.environs.manual")

type ProvisionMachineArgs struct {
	// Host is the SSH host: [user@]host
	Host string

	// DataDir is the root directory for juju data.
	// If left blank, the default location "/var/lib/juju" will be used.
	DataDir string

	// State is the *state.State object to register the machine with.
	State *state.State

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
// On successful completion, this function will return the state.Machine
// that was entered into state.
func ProvisionMachine(args ProvisionMachineArgs) (m *state.Machine, err error) {
	defer func() {
		if m != nil && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", m, err)
			m.EnsureDead()
			m.Remove()
			m = nil
		}
	}()

	var env environs.Environ
	if conn, err := juju.NewConnFromState(args.State); err != nil {
		return nil, err
	} else {
		env = conn.Environ
	}

	sshHostWithoutUser := args.Host
	if at := strings.Index(sshHostWithoutUser, "@"); at != -1 {
		sshHostWithoutUser = sshHostWithoutUser[at+1:]
	}
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

	provisioned, err := checkProvisioned(args.Host)
	if err != nil {
		err = fmt.Errorf("error checking if provisioned: %v", err)
		return nil, err
	}
	if provisioned {
		return nil, ErrProvisioned
	}

	hc, series, err := DetectSeriesAndHardwareCharacteristics(args.Host)
	if err != nil {
		err = fmt.Errorf("error detecting hardware characteristics: %v", err)
		return nil, err
	}

	tools := args.Tools
	if tools == nil {
		tools, err = findInstanceTools(env, series, *hc.Arch)
		if err != nil {
			return nil, err
		}
	}

	// Generate a unique nonce for the machine.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}

	// Inject a new machine into state.
	//
	// There will never be a corresponding "instance" that any provider
	// knows about. This is fine, and works well with the provisioner
	// task. The provisioner task will happily remove any and all dead
	// machines from state, but will ignore the associated instance ID
	// if it isn't one that the environment provider knows about.
	instanceId := instance.Id(manualInstancePrefix + sshHostWithoutUser)
	nonce := fmt.Sprintf("%s:%s", instanceId, uuid.String())
	m, err = injectMachine(injectMachineArgs{
		st:         args.State,
		instanceId: instanceId,
		addrs:      addrs,
		series:     series,
		hc:         hc,
		nonce:      nonce,
	})
	if err != nil {
		return nil, err
	}
	stateInfo, apiInfo, err := setupAuthentication(env, m)
	if err != nil {
		return m, err
	}

	// Finally, provision the machine agent.
	err = provisionMachineAgent(provisionMachineAgentArgs{
		host:          args.Host,
		dataDir:       args.DataDir,
		environConfig: env.Config(),
		machineId:     m.Id(),
		bootstrap:     false,
		nonce:         nonce,
		stateInfo:     stateInfo,
		apiInfo:       apiInfo,
		tools:         tools,
	})
	if err != nil {
		return m, err
	}

	logger.Infof("Provisioned machine %v", m)
	return m, nil
}

type injectMachineArgs struct {
	st         *state.State
	instanceId instance.Id
	addrs      []instance.Address
	series     string
	hc         instance.HardwareCharacteristics
	nonce      string
}

// injectMachine injects a machine into state with provisioned status.
func injectMachine(args injectMachineArgs) (m *state.Machine, err error) {
	defer func() {
		if m != nil && err != nil {
			logger.Errorf("injecting into state failed, removing machine %v: %v", m, err)
			m.EnsureDead()
			m.Remove()
		}
	}()
	m, err = args.st.InjectMachine(&state.AddMachineParams{
		Series:                  args.series,
		InstanceId:              args.instanceId,
		HardwareCharacteristics: args.hc,
		Nonce: args.nonce,
		Jobs:  []state.MachineJob{state.JobHostUnits},
	})
	if err != nil {
		return nil, err
	}
	if err = m.SetAddresses(args.addrs); err != nil {
		return nil, err
	}
	return m, nil
}

func setupAuthentication(env environs.Environ, m *state.Machine) (*state.Info, *api.Info, error) {
	auth, err := provisioner.NewEnvironAuthenticator(env)
	if err != nil {
		return nil, nil, err
	}
	return auth.SetupAuthentication(m)
}
