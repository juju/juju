// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
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

	// Env is the environment containing the state and API servers the
	// provisioned machine agent should communicate with.
	Env environs.Environ

	// State is the *state.State object to register the machine with.
	State *state.State

	// Constraints are any machine constraints that should be checked.
	Constraints constraints.Value

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
			m.EnsureDead()
			m.Remove()
			m = nil
		}
	}()

	sshHostWithoutUser := args.Host
	if at := strings.Index(sshHostWithoutUser, "@"); at != -1 {
		sshHostWithoutUser = sshHostWithoutUser[at+1:]
	}
	addrs, err := instance.HostAddresses(sshHostWithoutUser)
	if err != nil {
		return nil, err
	}

	provisioned, err := checkProvisioned(args.Host)
	if err != nil {
		err = fmt.Errorf("error checking if provisioned: %v", err)
		return nil, err
	}
	if provisioned {
		return nil, ErrProvisioned
	}

	hc, series, err := detectSeriesAndHardwareCharacteristics(args.Host)
	if err != nil {
		err = fmt.Errorf("error detecting hardware characteristics: %v", err)
		return nil, err
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
		env:        args.Env,
		st:         args.State,
		instanceId: instanceId,
		addrs:      addrs,
		series:     series,
		hc:         hc,
		cons:       args.Constraints,
		tools:      args.Tools,
		nonce:      nonce,
	})
	if err != nil {
		return nil, err
	}
	stateInfo, apiInfo, err := setupAuthentication(args.Env, m)
	if err != nil {
		return m, err
	}

	// Finally, provision the machine agent.
	err = provisionMachineAgent(provisionMachineAgentArgs{
		host:      args.Host,
		dataDir:   args.DataDir,
		envcfg:    args.Env.Config(),
		machine:   m,
		nonce:     nonce,
		stateInfo: stateInfo,
		apiInfo:   apiInfo,
		cons:      args.Constraints,
	})
	if err != nil {
		return m, err
	}

	logger.Infof("Provisioned machine %v", m)
	return m, nil
}

type injectMachineArgs struct {
	env        environs.Environ
	st         *state.State
	instanceId instance.Id
	addrs      []instance.Address
	series     string
	hc         instance.HardwareCharacteristics
	cons       constraints.Value
	tools      *tools.Tools
	nonce      string
}

// injectMachine injects a machine into state with provisioned status.
func injectMachine(args injectMachineArgs) (m *state.Machine, err error) {
	defer func() {
		if m != nil && err != nil {
			m.EnsureDead()
			m.Remove()
		}
	}()

	m, err = args.st.InjectMachine(&state.AddMachineParams{
		Series:                  args.series,
		Constraints:             args.cons,
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

	if args.tools == nil {
		// Normally, provisioning is done with configuration loaded from the
		// state database.  Manual provisioning be initiated from the juju CLI,
		// whose environment configuration may not reflect the configuration
		// in state; load configuration from state now.
		statecfg, err := args.st.EnvironConfig()
		if err != nil {
			return nil, err
		}
		if err = args.env.SetConfig(statecfg); err != nil {
			return nil, err
		}
		possibleTools, err := envtools.FindInstanceTools(args.env, args.series, args.cons)
		if err != nil {
			return nil, err
		}
		possibleTools, err = possibleTools.Match(tools.Filter{Arch: *args.hc.Arch})
		if err != nil {
			arches := possibleTools.Arches()
			return nil, fmt.Errorf("chosen architecture %v not present in %v", *args.hc.Arch, arches)
		}
		args.tools = possibleTools[0]
	}
	if err = m.SetAgentTools(args.tools); err != nil {
		return nil, err
	}

	return m, nil
}

func setupAuthentication(env environs.Environ, m *state.Machine) (*state.Info, *api.Info, error) {
	auth, err := provisioner.NewSimpleAuthenticator(env)
	if err != nil {
		return nil, nil, err
	}
	return auth.SetupAuthentication(m)
}
