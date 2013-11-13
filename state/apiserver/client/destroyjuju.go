// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"strings"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

// DestroyJuju cleanly removes all Juju agents from the environment.
func (c *Client) DestroyJuju() error {
	// First, set the environment to Dying, to lock out new machines,
	// services and units.
	env, err := c.api.state.Environment()
	if err != nil {
		return err
	}
	if err = env.Destroy(); err != nil {
		return err
	}
	// Destroy all services, preventing addition of units.
	services, err := c.api.state.AllServices()
	for _, s := range services {
		if err := s.Destroy(); err != nil {
			return err
		}
	}
	// Make sure there are no manually provisioned non-manager machines.
	// Manually provisioned manager machines can self-destruct when
	// environment goes to Dead.
	machines, err := c.api.state.AllMachines()
	if err != nil {
		return err
	}
	if err := checkManualMachines(machines); err != nil {
		return err
	}
	// We must destroy instances server-side to support hosted Juju,
	// as there's no CLI to fall back on. In that case, we only ever
	// destroy non-state machines; we leave destroying state servers
	// in non-hosted environments to the CLI, as otherwise the API
	// server may get cut off.
	if err := destroyInstances(c.api.state, machines); err != nil {
		return err
	}
	// Set the environment to Dead, which will cause all agents to
	// terminate and uninstall themselves. This must be the last thing
	// we do.
	return env.EnsureDead()
}

// destroyInstances directly destroys all non-manager machine instances.
func destroyInstances(st *state.State, machines []*state.Machine) error {
	var ids []instance.Id
	for _, m := range machines {
		if m.IsStateServer() {
			continue
		}
		id, err := m.InstanceId()
		if err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	envcfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	env, err := environs.New(envcfg)
	if err != nil {
		return err
	}
	instances, err := env.Instances(ids)
	switch err {
	case nil:
	default:
		return err
	case environs.ErrNoInstances:
		return nil
	case environs.ErrPartialInstances:
		var nonNilInstances []instance.Instance
		for _, inst := range instances {
			if inst == nil {
				continue
			}
			nonNilInstances = append(nonNilInstances, inst)
		}
		instances = nonNilInstances
	}
	return env.StopInstances(instances)
}

// checkManualMachines checks if any of the machines in the slice were
// manually provisioned, and are non-manager machines. These machines
// must (currently) by manually destroyed via destroy-machine before
// destroy-environment can successfully complete.
func checkManualMachines(machines []*state.Machine) error {
	var ids []string
	for _, m := range machines {
		if isManuallyProvisioned(m) && !m.IsStateServer() {
			ids = append(ids, m.Id())
		}
	}
	if len(ids) > 0 {
		return fmt.Errorf("manually provisioned machines must first be destroyed with `juju destroy-machine %s`", strings.Join(ids, " "))
	}
	return nil
}

// isManuallyProvisioned returns true iff the the machine was
// manually provisioned.
func isManuallyProvisioned(m *state.Machine) bool {
	iid, err := m.InstanceId()
	if err != nil {
		return false
	}
	// Due to an import loop in tests, we cannot import manual.
	return strings.HasPrefix(string(iid), "manual:")
}
