// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/version/v2"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

const (
	// ErrUpgradeStepsInvalidState is returned when the upgrade state is
	// invalid. We expect it to be in the db completed state, if that's not the
	// case, we can't proceed.
	ErrUpgradeStepsInvalidState = errors.ConstError("invalid upgrade state")

	// ErrFailedUpgradeSteps is returned when either controller fails to
	// complete its upgrade steps.
	ErrFailedUpgradeSteps = errors.ConstError("failed upgrade steps")

	// ErrUpgradeTimeout is returned when the upgrade steps fail to complete
	// within the timeout.
	ErrUpgradeTimeout = errors.ConstError("upgrade timeout")

	// defaultUpgradeTimeout is the default timeout for the upgrade to complete.
	// 10 minutes should be enough for the upgrade steps to complete.
	defaultUpgradeTimeout = 10 * time.Minute

	defaultRetryDelay    = 2 * time.Minute
	defaultRetryAttempts = 5
)

// NewLock creates a gate.Lock to be used to synchronise workers which
// need to start after upgrades have completed. The returned Lock should
// be passed to NewWorker. If the agent has already upgraded to the
// current version, then the lock will be returned in the released state.
func NewLock(agentConfig agent.Config) gate.Lock {
	lock := gate.NewLock()

	// Build numbers are irrelevant to upgrade steps.
	upgradedToVersion := agentConfig.UpgradedToVersion().ToPatch()
	currentVersion := jujuversion.Current.ToPatch()
	if upgradedToVersion == currentVersion {
		lock.Unlock()
	}

	return lock
}

type apiLostDuringUpgrade struct {
	err error
}

func (e *apiLostDuringUpgrade) Error() string {
	return fmt.Sprintf("API connection lost during upgrade: %v", e.err)
}

func isAPILostDuringUpgrade(err error) bool {
	_, ok := err.(*apiLostDuringUpgrade)
	return ok
}

type baseWorker struct {
	upgradeCompleteLock gate.Lock
	agent               agent.Agent
	apiCaller           base.APICaller
	statusSetter        StatusSetter

	preUpgradeSteps     PreUpgradeStepsFunc
	performUpgradeSteps UpgradeStepsFunc

	fromVersion version.Number
	toVersion   version.Number
	tag         names.Tag

	clock  clock.Clock
	logger Logger
}

func (w *baseWorker) alreadyUpgraded() bool {
	if w.upgradeCompleteLock.IsUnlocked() {
		// Our work is already done (we're probably being restarted
		// because the API connection has gone down), so do nothing.
		return true
	}

	if w.fromVersion == w.toVersion {
		w.logger.Infof("upgrade to %v already completed.", w.toVersion)
		w.upgradeCompleteLock.Unlock()
		return true
	}
	return false
}

// runUpgradeSteps runs the required upgrade steps for the agent,
// retrying on failure. The agent's UpgradedToVersion is set
// once the upgrade is complete.
//
// This function conforms to the agent.ConfigMutator type and is
// designed to be called via an agent's ChangeConfig method.
func (w *baseWorker) runUpgradeSteps(ctx context.Context, targets []upgrades.Target) func(agentConfig agent.ConfigSetter) error {
	return func(agentConfig agent.ConfigSetter) error {
		if err := w.statusSetter.SetStatus(status.Started, fmt.Sprintf("upgrading to %v", w.toVersion), nil); err != nil {
			return errors.Trace(err)
		}

		context := upgrades.NewContext(agentConfig, w.apiCaller)
		w.logger.Infof("starting upgrade from %v to %v for %q", w.fromVersion, w.toVersion, w.tag)

		retryStrategy := retry.CallArgs{
			Clock:    w.clock,
			Delay:    defaultRetryDelay,
			Attempts: defaultRetryAttempts,
			IsFatalError: func(err error) bool {
				// Abort if API connection has gone away!
				breakable, ok := w.apiCaller.(agenterrors.Breakable)
				if !ok {
					return false
				}
				return agenterrors.ConnectionIsDead(w.logger, breakable)
			},
			NotifyFunc: func(lastErr error, attempt int) {
				w.reportUpgradeFailure(lastErr, attempt == defaultRetryAttempts)
			},
			Func: func() error {
				return w.performUpgradeSteps(w.fromVersion, targets, context)
			},
			Stop: ctx.Done(),
		}

		err := retry.Call(retryStrategy)
		if retry.IsAttemptsExceeded(err) {
			return retry.LastError(err)
		}
		if err != nil {
			return &apiLostDuringUpgrade{err: err}
		}

		agentConfig.SetUpgradedToVersion(w.toVersion)
		return nil
	}
}

func (w *baseWorker) reportUpgradeFailure(err error, willRetry bool) {
	retryText := "will retry"
	if !willRetry {
		retryText = "giving up"
	}
	w.logger.Errorf("upgrade from %v to %v for %q failed (%s): %v",
		w.fromVersion, w.toVersion, w.tag, retryText, err)
	_ = w.statusSetter.SetStatus(status.Error,
		fmt.Sprintf("upgrade to %v failed (%s): %v", w.toVersion, retryText, err), nil)
}
