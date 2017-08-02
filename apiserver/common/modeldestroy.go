// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/apiserver/facades/agent/metricsender"
	"github.com/juju/juju/state"
)

var sendMetrics = func(st metricsender.ModelBackend) error {
	cfg, err := st.ModelConfig()
	if err != nil {
		return errors.Annotatef(err, "failed to get model config for %s", st.ModelTag())
	}

	err = metricsender.SendMetrics(
		st,
		metricsender.DefaultMetricSender(),
		clock.WallClock,
		metricsender.DefaultMaxBatchesPerSend(),
		cfg.TransmitVendorMetrics(),
	)
	return errors.Trace(err)
}

// DestroyController sets the controller model to Dying and, if requested,
// schedules cleanups so that all of the hosted models are destroyed, or
// otherwise returns an error indicating that there are hosted models
// remaining.
func DestroyController(
	st ModelManagerBackend,
	destroyHostedModels bool,
	destroyStorage *bool,
) error {
	modelTag := st.ModelTag()
	controllerModel, err := st.ControllerModel()
	if err != nil {
		return errors.Trace(err)
	}
	if modelTag != controllerModel.ModelTag() {
		return errors.Errorf(
			"expected state for controller model UUID %v, got %v",
			controllerModel.ModelTag().Id(),
			modelTag.Id(),
		)
	}
	if destroyHostedModels {
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
	}
	return destroyModel(st, state.DestroyModelParams{
		DestroyHostedModels: destroyHostedModels,
		DestroyStorage:      destroyStorage,
	})
}

// DestroyModel sets the model to Dying, such that the model's resources will
// be destroyed and the model removed from the controller.
func DestroyModel(
	st ModelManagerBackend,
	destroyStorage *bool,
) error {
	return destroyModel(st, state.DestroyModelParams{
		DestroyStorage: destroyStorage,
	})
}

func destroyModel(st ModelManagerBackend, args state.DestroyModelParams) error {
	check := NewBlockChecker(st)
	if err := check.DestroyAllowed(); err != nil {
		return errors.Trace(err)
	}

	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	if err := model.Destroy(args); err != nil {
		return errors.Trace(err)
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
