// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

const (
	// BootstrapNonce is used as a nonce for the state server machine.
	BootstrapNonce = "user-admin:bootstrap"
)

// InitializeState should be called on the bootstrap machine's agent
// configuration. It uses that information to create the state server, dial the
// state server, and initialize it. It also generates a new password for the
// bootstrap machine and calls Write to save the the configuration.
//
// The envCfg values will be stored in the state's EnvironConfig; the
// machineCfg values will be used to configure the bootstrap Machine,
// and its constraints will be also be used for the environment-level
// constraints. The connection to the state server will respect the
// given timeout parameter.
//
// InitializeState returns the newly initialized state and bootstrap
// machine. If it fails, the state may well be irredeemably compromised.
type StateInitializer interface {
	InitializeState(envCfg *config.Config, machineCfg BootstrapMachineConfig, timeout mongo.DialOpts, policy state.Policy) (*state.State, *state.Machine, error)
}

// BootstrapMachineConfig holds configuration information
// to attach to the bootstrap machine.
type BootstrapMachineConfig struct {
	// Addresses holds the bootstrap machine's addresses.
	Addresses []network.Address

	// Constraints holds the bootstrap machine's constraints.
	// This value is also used for the environment-level constraints.
	Constraints constraints.Value

	// Jobs holds the jobs that the machine agent will run.
	Jobs []params.MachineJob

	// InstanceId holds the instance id of the bootstrap machine.
	InstanceId instance.Id

	// Characteristics holds hardware information on the
	// bootstrap machine.
	Characteristics instance.HardwareCharacteristics

	// SharedSecret is the Mongo replica set shared secret (keyfile).
	SharedSecret string
}

const BootstrapMachineId = "0"

func InitializeState(c ConfigSetter, envCfg *config.Config, machineCfg BootstrapMachineConfig, dialOpts mongo.DialOpts, policy state.Policy) (_ *state.State, _ *state.Machine, resultErr error) {
	if c.Tag() != names.NewMachineTag(BootstrapMachineId) {
		return nil, nil, fmt.Errorf("InitializeState not called with bootstrap machine's configuration")
	}
	servingInfo, ok := c.StateServingInfo()
	if !ok {
		return nil, nil, fmt.Errorf("state serving information not available")
	}
	// N.B. no users are set up when we're initializing the state,
	// so don't use any tag or password when opening it.
	info, ok := c.MongoInfo()
	if !ok {
		return nil, nil, fmt.Errorf("stateinfo not available")
	}
	info.Tag = nil
	info.Password = c.OldPassword()

	if err := initMongoAdminUser(info.Info, dialOpts, info.Password); err != nil {
		return nil, nil, errors.Annotate(err, "failed to initialize mongo admin user")
	}

	logger.Debugf("initializing address %v", info.Addrs)
	st, err := state.Initialize(info, envCfg, dialOpts, policy)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize state: %v", err)
	}
	logger.Debugf("connected to initial state")
	defer func() {
		if resultErr != nil {
			st.Close()
		}
	}()
	servingInfo.SharedSecret = machineCfg.SharedSecret
	c.SetStateServingInfo(servingInfo)
	if err = initAPIHostPorts(c, st, machineCfg.Addresses, servingInfo.APIPort); err != nil {
		return nil, nil, err
	}
	if err := st.SetStateServingInfo(servingInfo); err != nil {
		return nil, nil, fmt.Errorf("cannot set state serving info: %v", err)
	}
	m, err := initConstraintsAndBootstrapMachine(c, st, machineCfg)
	if err != nil {
		return nil, nil, err
	}
	return st, m, nil
}

func initConstraintsAndBootstrapMachine(c ConfigSetter, st *state.State, cfg BootstrapMachineConfig) (*state.Machine, error) {
	if err := st.SetEnvironConstraints(cfg.Constraints); err != nil {
		return nil, fmt.Errorf("cannot set initial environ constraints: %v", err)
	}
	m, err := initBootstrapMachine(c, st, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize bootstrap machine: %v", err)
	}
	return m, nil
}

// initMongoAdminUser adds the admin user with the specified
// password to the admin database in Mongo.
func initMongoAdminUser(info mongo.Info, dialOpts mongo.DialOpts, password string) error {
	session, err := mongo.DialWithInfo(info, dialOpts)
	if err != nil {
		return err
	}
	defer session.Close()
	return mongo.SetAdminMongoPassword(session, mongo.AdminUser, password)
}

// initBootstrapMachine initializes the initial bootstrap machine in state.
func initBootstrapMachine(c ConfigSetter, st *state.State, cfg BootstrapMachineConfig) (*state.Machine, error) {
	logger.Infof("initialising bootstrap machine with config: %+v", cfg)

	jobs := make([]state.MachineJob, len(cfg.Jobs))
	for i, job := range cfg.Jobs {
		machineJob, err := state.MachineJobFromParams(job)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap machine job %q: %v", job, err)
		}
		jobs[i] = machineJob
	}
	m, err := st.AddOneMachine(state.MachineTemplate{
		Addresses:               cfg.Addresses,
		Series:                  version.Current.Series,
		Nonce:                   BootstrapNonce,
		Constraints:             cfg.Constraints,
		InstanceId:              cfg.InstanceId,
		HardwareCharacteristics: cfg.Characteristics,
		Jobs: jobs,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create bootstrap machine in state: %v", err)
	}
	if m.Id() != BootstrapMachineId {
		return nil, fmt.Errorf("bootstrap machine expected id 0, got %q", m.Id())
	}
	// Read the machine agent's password and change it to
	// a new password (other agents will change their password
	// via the API connection).
	logger.Debugf("create new random password for machine %v", m.Id())

	newPassword, err := utils.RandomPassword()
	if err != nil {
		return nil, err
	}
	if err := m.SetPassword(newPassword); err != nil {
		return nil, err
	}
	if err := m.SetMongoPassword(newPassword); err != nil {
		return nil, err
	}
	c.SetPassword(newPassword)
	return m, nil
}

// initAPIHostPorts sets the initial API host/port addresses in state.
func initAPIHostPorts(c ConfigSetter, st *state.State, addrs []network.Address, apiPort int) error {
	hostPorts := network.AddressesWithPort(addrs, apiPort)
	return st.SetAPIHostPorts([][]network.HostPort{hostPorts})
}
