// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"time"

	coreerrors "launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
)

// DestroyJuju cleanly removes all Juju agents from the environment.
func (c *Client) DestroyJuju(args params.DestroyJuju) error {
	// First, set the environment to Dying, to lock out new machines,
	// services and units.
	env, err := c.api.state.Environment()
	if err != nil {
		return err
	}
	if err = env.Destroy(); err != nil {
		return err
	}
	// Now destroy all existing units, and wait for them to be removed.
	attemptStrategy := utils.AttemptStrategy{
		Total: args.Timeout,
		Delay: 1 * time.Second,
	}
	if attemptStrategy.Total < attemptStrategy.Delay {
		attemptStrategy.Delay = attemptStrategy.Total
	}
	if err = destroyUnits(c.api.state, attemptStrategy); err != nil {
		return err
	}
	// Finally, set the environment to Dead, which will cause
	// all agents to terminate and uninstall themselves.
	return env.EnsureDead()
}

// destroyUnits destroys all of the principal units on the specified
// machines, and waits for them to be removed from state.
//
// The supplied AttemptStrategy governs how long the entire operation
// may take overall (attempt.Total), and how long to wait before
// reattempting each individual action (attempt.Delay).
func destroyUnits(st *state.State, attemptStrategy utils.AttemptStrategy) error {
	machines, err := st.AllMachines()
	if err != nil {
		return err
	}
	attempt := attemptStrategy.Start()
	// First, advance all units to Dying.
	var allUnits []*state.Unit
	for _, m := range machines {
		logger.Infof("destroying units on %v", m.Tag())
		units, err := m.Units()
		if err != nil {
			logger.Errorf("failed to list units on %v: %v", m.Tag(), err)
			return err
		}
		for _, u := range units {
			if !u.IsPrincipal() {
				continue
			}
			logger.Infof("destroying %v", u.Tag())
			if err := u.Destroy(); err != nil {
				logger.Errorf("failed to destroy %v: %v", m.Tag(), err)
				return err
			}
			allUnits = append(allUnits, u)
		}
	}
	// Now wait for the units to be removed from state.
	for _, u := range allUnits {
		logger.Infof("waiting for %v to die", u.Tag())
		if !waitNotFound(u.Refresh, attempt) {
			return fmt.Errorf("%s was not removed from state in a timely manner", u.Tag())
		}
	}
	return nil
}

// waitNotFound calls the provided function in a loop, until either the
// function returns an error that satisfies errors.IsNotFoundError, or no
// more attempts are allowed.
//
// waitNotFound returns true if the function returned a satisfying error,
// and false otherwise.
func waitNotFound(f func() error, attempt *utils.Attempt) bool {
	for {
		if err := f(); coreerrors.IsNotFoundError(err) {
			return true
		}
		if !attempt.Next() {
			return false
		}
	}
}
