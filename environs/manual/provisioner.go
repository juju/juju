// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
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

	// LogDir is the log directory for juju.
	// If left blank, the default location "/var/log/juju" will be used.
	LogDir string

	// Env is the environment containing the state and API servers the
	// provisioned machine agent should communicate with.
	Env environs.Environ

	// State is the *state.State object to register the machine with.
	State *state.State

	// Constraints are any machine constraints that should be checked.
	Constraints constraints.Value

	// Tools to install on the machine. If nil, the bootstrap machine's
	// tools will be used.
	Tools *tools.Tools
}

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
		return nil, err
	}

	// Finally, provision the machine agent.
	err = provisionMachineAgent(provisionMachineAgentArgs{
		host:      args.Host,
		dataDir:   args.DataDir,
		logDir:    args.LogDir,
		envcfg:    args.Env.Config(),
		machine:   m,
		nonce:     nonce,
		stateInfo: stateInfo,
		apiInfo:   apiInfo,
	})
	if err != nil {
		return nil, err
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

	m, err = args.st.InjectMachine(
		args.series,
		args.cons,
		args.instanceId,
		args.hc,
		args.nonce,
		state.JobHostUnits,
	)
	if err != nil {
		return nil, err
	}
	if err = m.SetAddresses(args.addrs); err != nil {
		return nil, err
	}

	// We can't use environs.FindInstanceTools, as it chooses the tools based
	// on the version of the juju tool executing, which might not even exist in
	// storage. Set the new machine's tools to be the same as those of the
	// bootstrap machine's.
	tools := args.tools
	if tools == nil {
		bootstrapMachine, err := args.st.Machine("0")
		if err != nil {
			return nil, err
		}
		tools, err = bootstrapMachine.AgentTools()
		if err != nil {
			return nil, err
		}
		if err = m.SetAgentTools(tools); err != nil {
			return nil, err
		}
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
