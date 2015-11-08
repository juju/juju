// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/state"
)

var sendMetrics = func(st *state.State) error {
	err := metricsender.SendMetrics(st, metricsender.DefaultMetricSender(), metricsender.DefaultMaxBatchesPerSend())
	return errors.Trace(err)
}

// DestroyEnvironment sets the environment to dying. Cleanup jobs then destroy
// all services and non-manager machine instances in the specified
// environment. This function assumes that all necessary authentication checks
// have been done.
func DestroyEnvironment(st *state.State, environTag names.EnvironTag, destroyHostedEnvirons bool) error {
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

	if destroyHostedEnvirons {
		if err := env.DestroyIncludingHosted(); err != nil {
			return err
		}
	} else {
		if err = env.Destroy(); err != nil {
			return errors.Trace(err)
		}
	}

	err = sendMetrics(st)
	if err != nil {
		logger.Warningf("failed to send leftover metrics: %v", err)
	}

	// Return to the caller. If it's the CLI, it will finish up by calling the
	// provider's Destroy method, which will destroy the state servers, any
	// straggler instances, and other provider-specific resources. The Reaper
	// worker will teardown all resources. After which, the Undertaker worker
	// handles the removal of the environment.
	return nil
}
