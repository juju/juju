// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facades/agent/metricsender"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

var sendMetrics = func(st metricsender.ModelBackend) error {
	ccfg, err := st.ControllerConfig()
	if err != nil {
		return errors.Annotate(err, "failed to get controller config")
	}
	meteringURL := ccfg.MeteringURL()
	cfg, err := st.ModelConfig()
	if err != nil {
		return errors.Annotatef(err, "failed to get model config for %s", st.ModelTag())
	}

	err = metricsender.SendMetrics(
		st,
		metricsender.DefaultSenderFactory()(meteringURL),
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
	force *bool,
	maxWait *time.Duration,
	modelTimeout *time.Duration,
) error {
	modelTag := st.ModelTag()
	controllerModelTag := st.ControllerModelTag()
	if modelTag != controllerModelTag {
		return errors.Errorf(
			"expected state for controller model UUID %v, got %v",
			controllerModelTag.Id(),
			modelTag.Id(),
		)
	}
	if destroyHostedModels {
		uuids, err := st.AllModelUUIDs()
		if err != nil {
			return errors.Trace(err)
		}
		for _, uuid := range uuids {
			modelSt, release, err := st.GetBackend(uuid)
			if err != nil {
				if errors.IsNotFound(err) {
					// Model is already in the process of being destroyed.
					continue
				}
				return errors.Trace(err)
			}
			defer release()

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
		Force:               force,
		MaxWait:             MaxWait(maxWait),
		Timeout:             modelTimeout,
	})
}

// DestroyModel sets the model to Dying, such that the model's resources will
// be destroyed and the model removed from the controller.
func DestroyModel(
	st ModelManagerBackend,
	destroyStorage *bool,
	force *bool,
	maxWait *time.Duration,
	timeout *time.Duration,
) error {
	return destroyModel(st, state.DestroyModelParams{
		DestroyStorage: destroyStorage,
		Force:          force,
		MaxWait:        MaxWait(maxWait),
		Timeout:        timeout,
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
	notForcing := args.Force == nil || !*args.Force
	if notForcing {
		// If model status is suspended, then model's cloud credential is invalid.
		modelStatus, err := model.Status()
		if err != nil {
			return errors.Trace(err)
		}
		if modelStatus.Status == status.Suspended {
			return errors.Errorf("invalid cloud credential, use --force")
		}
	}
	if err := model.Destroy(args); err != nil {
		return errors.Annotate(err, "initiating destroy model operation")
	}

	err = sendMetrics(st)
	if err != nil {
		logger.Errorf("failed to send leftover metrics: %v", err)
	}

	// Return to the caller. If it's the CLI, it will finish up by calling the
	// provider's Destroy method, which will destroy the controllers, any
	// straggler instances, and other provider-specific resources. Once all
	// resources are torn down, the Undertaker worker handles the removal of
	// the model.
	return nil
}
