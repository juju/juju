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

// DestroyModelIncludingHosted sets the model to dying. Cleanup jobs then destroy
// all services and non-manager, non-manual machine instances in the specified
// model. This function assumes that all necessary authentication checks
// have been done. If the model is a controller hosting other
// models, they will also be destroyed.
func DestroyModelIncludingHosted(st *state.State, modelTag names.ModelTag) error {
	return destroyModel(st, modelTag, true)
}

// DestroyModel sets the environment to dying. Cleanup jobs then destroy
// all services and non-manager, non-manual machine instances in the specified
// model. This function assumes that all necessary authentication checks
// have been done. An error will be returned if this model is a
// controller hosting other model.
func DestroyModel(st *state.State, modelTag names.ModelTag) error {
	return destroyModel(st, modelTag, false)
}

func destroyModel(st *state.State, modelTag names.ModelTag, destroyHostedModels bool) error {
	var err error
	if modelTag != st.ModelTag() {
		if st, err = st.ForModel(modelTag); err != nil {
			return errors.Trace(err)
		}
		defer st.Close()
	}

	if destroyHostedModels {
		envs, err := st.AllModels()
		if err != nil {
			return errors.Trace(err)
		}
		for _, env := range envs {
			envSt, err := st.ForModel(env.ModelTag())
			defer envSt.Close()
			if err != nil {
				return errors.Trace(err)
			}
			check := NewBlockChecker(envSt)
			if err = check.DestroyAllowed(); err != nil {
				return errors.Trace(err)
			}
		}
	} else {
		check := NewBlockChecker(st)
		if err = check.DestroyAllowed(); err != nil {
			return errors.Trace(err)
		}
	}

	env, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	if destroyHostedModels {
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
	// provider's Destroy method, which will destroy the controllers, any
	// straggler instances, and other provider-specific resources. Once all
	// resources are torn down, the Undertaker worker handles the removal of
	// the environment.
	return nil
}
