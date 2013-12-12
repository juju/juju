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

// DestroyEnvironment destroys all services and non-manager machine
// instances in the environment.
func (c *Client) DestroyEnvironment() error {
	// TODO(axw) 2013-08-30 bug 1218688
	//
	// There's a race here: a client might add a manual machine
	// after another client checks. Destroy-environment will
	// still fail, but the environment will be in a state where
	// entities can only be destroyed. In the future we should
	// introduce a method of preventing Environment.Destroy()
	// from succeeding if machines have been added.

	// First, check for manual machines. We bail out if there are any,
	// to stop the user from prematurely hobbling the environment.
	machines, err := c.api.state.AllMachines()
	if err != nil {
		return err
	}
	if err := checkManualMachines(machines); err != nil {
		return err
	}

	// Set the environment to Dying, to lock out new machines and services.
	// Environment.Destroy() also schedules a cleanup for existing services.
	env, err := c.api.state.Environment()
	if err != nil {
		return err
	}
	if err = env.Destroy(); err != nil {
		return err
	}

	// Refresh machines and make sure once again that there are no
	// manually provisioned non-manager machines. This caters for
	// the race between the first check and the Environment.Destroy().
	machines, err = c.api.state.AllMachines()
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

	// Return to the caller. If it's the CLI, it will finish up
	// by calling the provider's Destroy method, which will
	// destroy the state servers, any straggler instances, and
	// other provider-specific resources.
	return nil
}

// destroyInstances directly destroys all non-manager machine instances.
func destroyInstances(st *state.State, machines []*state.Machine) error {
	var ids []instance.Id
	for _, m := range machines {
		if m.IsManager() {
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
	// TODO(axw) 2013-12-12 #1260171
	// Modify InstanceBroker.StopInstances to take
	// a slice of IDs rather than Instances.
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
		if isManuallyProvisioned(m) && !m.IsManager() {
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
