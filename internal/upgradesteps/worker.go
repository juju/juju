// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"
	"fmt"

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
	"github.com/juju/juju/worker/gate"
)

type (
	PreUpgradeStepsFunc = upgrades.PreUpgradeStepsFunc
	UpgradeStepsFunc    = upgrades.UpgradeStepsFunc
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Errorf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
	Debugf(string, ...any)
}

// StatusSetter defines the single method required to set an agent's
// status.
type StatusSetter interface {
	SetStatus(setableStatus status.Status, info string, data map[string]any) error
}

// BaseWorker defines the common fields and methods used by the
// machine and controller upgrade workers.
type BaseWorker struct {
	UpgradeCompleteLock gate.Lock
	Agent               agent.Agent
	APICaller           base.APICaller
	StatusSetter        StatusSetter

	PreUpgradeSteps     PreUpgradeStepsFunc
	PerformUpgradeSteps UpgradeStepsFunc

	FromVersion version.Number
	ToVersion   version.Number
	Tag         names.Tag

	Clock  clock.Clock
	Logger Logger
}

func (w *BaseWorker) AlreadyUpgraded() bool {
	if w.UpgradeCompleteLock.IsUnlocked() {
		// Our work is already done (we're probably being restarted
		// because the API connection has gone down), so do nothing.
		return true
	}

	if w.FromVersion == w.ToVersion {
		w.Logger.Infof("upgrade to %v already completed.", w.ToVersion)
		w.UpgradeCompleteLock.Unlock()
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
func (w *BaseWorker) RunUpgradeSteps(ctx context.Context, targets []upgrades.Target) func(agentConfig agent.ConfigSetter) error {
	return func(agentConfig agent.ConfigSetter) error {
		if err := w.StatusSetter.SetStatus(status.Started, fmt.Sprintf("upgrading to %v", w.ToVersion), nil); err != nil {
			return errors.Trace(err)
		}

		context := upgrades.NewContext(agentConfig, w.APICaller)
		w.Logger.Infof("starting upgrade from %v to %v for %q", w.FromVersion, w.ToVersion, w.Tag)

		retryStrategy := retry.CallArgs{
			Clock:    w.Clock,
			Delay:    DefaultRetryDelay,
			Attempts: DefaultRetryAttempts,
			IsFatalError: func(err error) bool {
				// Abort if API connection has gone away!
				breakable, ok := w.APICaller.(agenterrors.Breakable)
				if !ok {
					return false
				}
				return agenterrors.ConnectionIsDead(w.Logger, breakable)
			},
			NotifyFunc: func(lastErr error, attempt int) {
				w.reportUpgradeFailure(lastErr, attempt == DefaultRetryAttempts)
			},
			Func: func() error {
				return w.PerformUpgradeSteps(w.FromVersion, targets, context)
			},
			Stop: ctx.Done(),
		}

		err := retry.Call(retryStrategy)
		if retry.IsAttemptsExceeded(err) {
			return retry.LastError(err)
		}
		if err != nil {
			return &APILostDuringUpgrade{err: err}
		}

		agentConfig.SetUpgradedToVersion(w.ToVersion)
		return nil
	}
}

func (w *BaseWorker) reportUpgradeFailure(err error, willRetry bool) {
	retryText := "will retry"
	if !willRetry {
		retryText = "giving up"
	}
	w.Logger.Errorf("upgrade from %v to %v for %q failed (%s): %v",
		w.FromVersion, w.ToVersion, w.Tag, retryText, err)
	_ = w.StatusSetter.SetStatus(status.Error,
		fmt.Sprintf("upgrade to %v failed (%s): %v", w.ToVersion, retryText, err), nil)
}
