// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

var sendMetrics = func(st *state.State) error {
	err := metricsender.SendMetrics(st, metricsender.DefaultMetricSender(), metricsender.DefaultMaxBatchesPerSend())
	return errors.Trace(err)
}

// DestroyEnvironment destroys all services and non-manager machine
// instances in the specified environment. This function assumes that all
// necessary authentication checks have been done.
func DestroyEnvironment(st *state.State, environTag names.EnvironTag) error {
	var err error
	if environTag != st.EnvironTag() {
		if st, err = st.ForEnviron(environTag); err != nil {
			return errors.Trace(err)
		}
		defer st.Close()
	}

	check := NewBlockChecker(st)
	if err = check.DestroyAllowed(); err != nil {
		return errors.Trace(err)
	}

	env, err := st.Environment()
	if err != nil {
		return errors.Trace(err)
	}

	if err = env.Destroy(); err != nil {
		return errors.Trace(err)
	}

	machines, err := st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}

	err = sendMetrics(st)
	if err != nil {
		logger.Warningf("failed to send leftover metrics: %v", err)
	}

	// We must destroy instances server-side to support JES (Juju Environment
	// Server), as there's no CLI to fall back on. In that case, we only ever
	// destroy non-state machines; we leave destroying state servers in non-
	// hosted environments to the CLI, as otherwise the API server may get cut
	// off.
	if err := destroyNonManagerMachines(st, machines); err != nil {
		return errors.Trace(err)
	}

	// If this is not the state server environment, remove all documents from
	// state associated with the environment.
	if env.EnvironTag() != env.ServerTag() {
		return errors.Trace(st.RemoveAllEnvironDocs())
	}

	// Return to the caller. If it's the CLI, it will finish up
	// by calling the provider's Destroy method, which will
	// destroy the state servers, any straggler instances, and
	// other provider-specific resources.
	return nil
}

// destroyNonManagerMachines directly destroys all non-manager, non-manual
// machine instances.
func destroyNonManagerMachines(st *state.State, machines []*state.Machine) error {
	var ids []instance.Id
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		if _, isContainer := m.ParentId(); isContainer {
			continue
		}
		manual, err := m.IsManual()
		if err != nil {
			return err
		} else if manual {
			continue
		}
		// There is a possible race here if a machine is being
		// provisioned, but hasn't yet come up.
		id, err := m.InstanceId()
		if err != nil {
			continue
		}
		ids = append(ids, id)
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
	return env.StopInstances(ids...)
}
