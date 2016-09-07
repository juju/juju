// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/metricsender"
)

var sendMetrics = func(st metricsender.ModelBackend) error {
	cfg, err := st.ModelConfig()
	if err != nil {
		return errors.Annotatef(err, "failed to get model config for %s", st.ModelTag())
	}

	err = metricsender.SendMetrics(st, metricsender.DefaultMetricSender(), clock.WallClock, metricsender.DefaultMaxBatchesPerSend(), cfg.TransmitVendorMetrics())
	return errors.Trace(err)
}

// DestroyModelIncludingHosted sets the model to dying. Cleanup jobs then destroy
// all services and non-manager, non-manual machine instances in the specified
// model. This function assumes that all necessary authentication checks
// have been done. If the model is a controller hosting other
// models, they will also be destroyed.
func DestroyModelIncludingHosted(st ModelManagerBackend, systemTag names.ModelTag) error {
	return destroyModel(st, systemTag, true)
}

// DestroyModel sets the environment to dying. Cleanup jobs then destroy
// all services and non-manager, non-manual machine instances in the specified
// model. This function assumes that all necessary authentication checks
// have been done. An error will be returned if this model is a
// controller hosting other model.
func DestroyModel(st ModelManagerBackend, modelTag names.ModelTag) error {
	return destroyModel(st, modelTag, false)
}

func destroyModel(st ModelManagerBackend, modelTag names.ModelTag, destroyHostedModels bool) error {
	var err error
	if modelTag != st.ModelTag() {
		if st, err = st.ForModel(modelTag); err != nil {
			return errors.Trace(err)
		}
		defer st.Close()
	}

	if destroyHostedModels {
		// Check we are operating on the controller state.
		controllerModel, err := st.ControllerModel()
		if err != nil {
			return errors.Trace(err)
		}
		if modelTag != controllerModel.ModelTag() {
			return errors.Errorf("expected controller model UUID %v, got %v", modelTag.Id(), controllerModel.ModelTag().Id())
		}
		models, err := st.AllModels()
		if err != nil {
			return errors.Trace(err)
		}
		for _, model := range models {
			modelSt, err := st.ForModel(model.ModelTag())
			defer modelSt.Close()
			if err != nil {
				return errors.Trace(err)
			}
			check := NewBlockChecker(modelSt)
			if err = check.DestroyAllowed(); err != nil {
				return errors.Trace(err)
			}
			err = sendMetrics(modelSt)
			if err != nil {
				logger.Errorf("failed to send leftover metrics: %v", err)
			}

		}
	} else {
		check := NewBlockChecker(st)
		if err = check.DestroyAllowed(); err != nil {
			return errors.Trace(err)
		}
	}

	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	if destroyHostedModels {
		if err := model.DestroyIncludingHosted(); err != nil {
			return err
		}
	} else {
		if err = model.Destroy(); err != nil {
			return errors.Trace(err)
		}
	}
	err = sendMetrics(st)
	if err != nil {
		logger.Errorf("failed to send leftover metrics: %v", err)
	}

	// Return to the caller. If it's the CLI, it will finish up by calling the
	// provider's Destroy method, which will destroy the controllers, any
	// straggler instances, and other provider-specific resources. Once all
	// resources are torn down, the Undertaker worker handles the removal of
	// the environment.
	return nil
}
