// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

// MaxWait is how far in the future the backstop force cleanup will be scheduled.
// Default is 1min if no value is provided.
func MaxWait(in *time.Duration) time.Duration {
	if in != nil {
		return *in
	}
	return 1 * time.Minute
}

// DestroyController sets the controller model to Dying and, if requested,
// schedules cleanups so that all of the hosted models are destroyed, or
// otherwise returns an error indicating that there are hosted models
// remaining.
func DestroyController(
	ctx context.Context,
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
				if errors.Is(err, errors.NotFound) {
					// Model is already in the process of being destroyed.
					continue
				}
				return errors.Trace(err)
			}
			defer release()

			check := NewBlockChecker(modelSt)
			if err = check.DestroyAllowed(ctx); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return destroyModel(ctx, st, state.DestroyModelParams{
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
	ctx context.Context,
	st ModelManagerBackend,
	destroyStorage *bool,
	force *bool,
	maxWait *time.Duration,
	timeout *time.Duration,
) error {
	return destroyModel(ctx, st, state.DestroyModelParams{
		DestroyStorage: destroyStorage,
		Force:          force,
		MaxWait:        MaxWait(maxWait),
		Timeout:        timeout,
	})
}

func destroyModel(ctx context.Context, st ModelManagerBackend, args state.DestroyModelParams) error {
	check := NewBlockChecker(st)
	if err := check.DestroyAllowed(ctx); err != nil {
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
		if notForcing {
			return errors.Trace(err)
		}
		logger.Warningf("failed destroying model %v: %v", model.UUID(), err)
		if err := filterNonCriticalErrorForForce(err); err != nil {
			return errors.Trace(err)
		}
	}

	// Return to the caller. If it's the CLI, it will finish up by calling the
	// provider's Destroy method, which will destroy the controllers, any
	// straggler instances, and other provider-specific resources. Once all
	// resources are torn down, the Undertaker worker handles the removal of
	// the model.
	return nil
}

func filterNonCriticalErrorForForce(err error) error {
	if errors.Is(err, stateerrors.PersistentStorageError) {
		return err
	}
	return nil
}
