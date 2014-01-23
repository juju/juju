// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// InitializeState should be called on the bootstrap machine's agent
// configuration. It uses that information to dial the state server and
// initialize it. It also generates a new password for the bootstrap
// machine and calls Write to save the the configuration.
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
	InitializeState(envCfg *config.Config, machineCfg BootstrapMachineConfig, timeout state.DialOpts) (*state.State, *state.Machine, error)
}

// BootstrapMachineConfig holds configuration information
// to attach to the bootstrap machine.
type BootstrapMachineConfig struct {
	// Constraints holds the bootstrap machine's constraints.
	// This value is also used for the environment-level constraints.
	Constraints constraints.Value

	// Jobs holds the jobs that the machine agent will run.
	Jobs []state.MachineJob

	// InstanceId holds the instance id of the bootstrap machine.
	InstanceId instance.Id

	// Characteristics holds hardware information on the
	// bootstrap machine.
	Characteristics instance.HardwareCharacteristics
}

const bootstrapMachineId = "0"

func (c *configInternal) InitializeState(envCfg *config.Config, machineCfg BootstrapMachineConfig, timeout state.DialOpts) (*state.State, *state.Machine, error) {
	if c.Tag() != names.MachineTag(bootstrapMachineId) {
		return nil, nil, fmt.Errorf("InitializeState not called with bootstrap machine's configuration")
	}
	info := state.Info{
		Addrs:  c.stateDetails.addresses,
		CACert: c.caCert,
	}
	logger.Debugf("initializing address %v", info.Addrs)
	st, err := state.Initialize(&info, envCfg, timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize state: %v", err)
	}
	logger.Debugf("connected to initial state")
	m, err := c.initUsersAndBootstrapMachine(st, machineCfg)
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	return st, m, nil
}

func (c *configInternal) initUsersAndBootstrapMachine(st *state.State, cfg BootstrapMachineConfig) (*state.Machine, error) {
	if err := initBootstrapUser(st, c.oldPassword); err != nil {
		return nil, fmt.Errorf("cannot initialize bootstrap user: %v", err)
	}
	if err := st.SetEnvironConstraints(cfg.Constraints); err != nil {
		return nil, fmt.Errorf("cannot set initial environ constraints: %v", err)
	}
	m, err := c.initBootstrapMachine(st, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize bootstrap machine: %v", err)
	}
	return m, nil
}

// initBootstrapUser creates the initial admin user for the database, and sets
// the initial password.
func initBootstrapUser(st *state.State, passwordHash string) error {
	logger.Debugf("adding admin user")
	// Set up initial authentication.
	u, err := st.AddUser("admin", "")
	if err != nil {
		return err
	}

	// Note that at bootstrap time, the password is set to
	// the hash of its actual value. The first time a client
	// connects to mongo, it changes the mongo password
	// to the original password.
	logger.Debugf("setting password hash for admin user")
	// TODO(jam): http://pad.lv/1248839
	// We could teach bootstrap how to generate a custom salt and apply
	// that to the hash that was generated. At which point we'd need to set
	// it here. For now, we pass "" so that on first login we will create a
	// new salt, but the fixed-salt password is still available from
	// cloud-init.
	if err := u.SetPasswordHash(passwordHash, ""); err != nil {
		return err
	}
	if err := st.SetAdminMongoPassword(passwordHash); err != nil {
		return err
	}
	return nil
}

// initBootstrapMachine initializes the initial bootstrap machine in state.
func (c *configInternal) initBootstrapMachine(st *state.State, cfg BootstrapMachineConfig) (*state.Machine, error) {

	logger.Infof("initialising bootstrap machine with config: %+v", cfg)

	m, err := st.AddOneMachine(state.MachineTemplate{
		Series:                  version.Current.Series,
		Nonce:                   state.BootstrapNonce,
		Constraints:             cfg.Constraints,
		InstanceId:              cfg.InstanceId,
		HardwareCharacteristics: cfg.Characteristics,
		Jobs: cfg.Jobs,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create bootstrap machine in state: %v", err)
	}
	if m.Id() != bootstrapMachineId {
		return nil, fmt.Errorf("bootstrap machine expected id 0, got %q", m.Id())
	}
	// Read the machine agent's password and change it to
	// a new password (other agents will change their password
	// via the API connection).
	logger.Debugf("create new random password for machine %v", m.Id())

	newPassword, err := c.writeNewPassword()
	if err != nil {
		return nil, err
	}
	if err := m.SetMongoPassword(newPassword); err != nil {
		return nil, err
	}
	if err := m.SetPassword(newPassword); err != nil {
		return nil, err
	}
	return m, nil
}
