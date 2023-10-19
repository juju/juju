// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	"github.com/juju/juju/internal/wrench"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

const (
	// ErrUpgradeStepsInvalidState is returned when the upgrade state is
	// invalid. We expect it to be in the db completed state, if that's not the
	// case, we can't proceed.
	ErrUpgradeStepsInvalidState = errors.ConstError("invalid upgrade state")

	// defaultUpgradeTimeout is the default timeout for the upgrade to complete.
	// 10 minutes should be enough for the upgrade steps to complete.
	defaultUpgradeTimeout = 10 * time.Minute

	defaultRetryDelay    = 2 * time.Minute
	defaultRetryAttempts = 5
)

type PerformUpgradeFunc func(fromVersion version.Number, targets []upgrades.Target, context upgrades.Context) error

// NewLock creates a gate.Lock to be used to synchronise workers which
// need to start after upgrades have completed. The returned Lock should
// be passed to NewWorker. If the agent has already upgraded to the
// current version, then the lock will be returned in the released state.
func NewLock(agentConfig agent.Config) gate.Lock {
	lock := gate.NewLock()

	if wrench.IsActive(wrenchKey(agentConfig), "always-try-upgrade") {
		// Always enter upgrade mode. This allows test of upgrades
		// even when there's actually no upgrade steps to run.
		return lock
	}

	// Build numbers are irrelevant to upgrade steps.
	upgradedToVersion := agentConfig.UpgradedToVersion().ToPatch()
	currentVersion := jujuversion.Current.ToPatch()
	if upgradedToVersion == currentVersion {
		loggo.GetLogger("juju.worker.upgradesteps").Infof(
			"upgrade steps for %v have already been run.",
			jujuversion.Current,
		)
		lock.Unlock()
	}

	return lock
}

// StatusSetter defines the single method required to set an agent's
// status.
type StatusSetter interface {
	SetStatus(setableStatus status.Status, info string, data map[string]any) error
}

// UpgradeService is the interface for the upgrade service.
type UpgradeService interface {
	// SetControllerDone marks the supplied controllerID as having
	// completed its upgrades. When SetControllerDone is called by the
	// last provisioned controller, the upgrade will be archived.
	SetControllerDone(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error
	// ActiveUpgrade returns the uuid of the current active upgrade.
	// If there are no active upgrades, return a NotFound error
	ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error)
	// // UpgradeInfo returns the upgrade info for the supplied upgradeUUID.
	UpgradeInfo(ctx context.Context, upgradeUUID domainupgrade.UUID) (upgrade.Info, error)
	// WatchForUpgradeState creates a watcher which notifies when the upgrade
	// has reached the given state.
	WatchForUpgradeState(ctx context.Context, upgradeUUID domainupgrade.UUID, state upgrade.State) (watcher.NotifyWatcher, error)
}

// NewWorker returns a new instance of the upgradeSteps worker. It
// will run any required steps to upgrade to the currently running
// Juju version.
func NewWorker(
	upgradeCompleteLock gate.Lock,
	agent agent.Agent,
	apiCaller base.APICaller,
	upgradeService UpgradeService,
	isController bool,
	tag names.Tag,
	preUpgradeSteps upgrades.PreUpgradeStepsFunc,
	performUpgradeSteps upgrades.UpgradeStepsFunc,
	entity StatusSetter,
	logger Logger,
) (worker.Worker, error) {
	return newWorker(
		upgradeCompleteLock,
		agent,
		apiCaller,
		upgradeService,
		isController,
		tag,
		preUpgradeSteps,
		performUpgradeSteps,
		entity,
		logger,
		defaultRetryDelay,
		defaultRetryAttempts,
	)
}

func newWorker(
	upgradeCompleteLock gate.Lock,
	agent agent.Agent,
	apiCaller base.APICaller,
	upgradeService UpgradeService,
	isController bool,
	tag names.Tag,
	preUpgradeSteps PreUpgradeStepsFunc,
	performUpgradeSteps UpgradeStepsFunc,
	entity StatusSetter,
	logger Logger,
	retryDelay time.Duration,
	retryAttempts int,
) (*upgradeSteps, error) {
	w := &upgradeSteps{
		upgradeCompleteLock: upgradeCompleteLock,
		agent:               agent,
		apiCaller:           apiCaller,
		upgradeService:      upgradeService,
		preUpgradeSteps:     preUpgradeSteps,
		performUpgradeSteps: performUpgradeSteps,
		entity:              entity,
		tag:                 tag,
		isController:        isController,
		logger:              logger,
		retryDelay:          retryDelay,
		retryAttempts:       retryAttempts,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.run,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type upgradeSteps struct {
	catacomb            catacomb.Catacomb
	upgradeCompleteLock gate.Lock
	agent               agent.Agent
	apiCaller           base.APICaller
	upgradeService      UpgradeService
	entity              StatusSetter

	preUpgradeSteps     PreUpgradeStepsFunc
	performUpgradeSteps UpgradeStepsFunc

	fromVersion version.Number
	toVersion   version.Number
	tag         names.Tag
	// If the agent is a machine agent for a controller, flag that state
	// needs to be opened before running upgrade steps
	isController bool

	clock  clock.Clock
	logger Logger

	retryDelay    time.Duration
	retryAttempts int
}

// Kill is part of the worker.Worker interface.
func (w *upgradeSteps) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *upgradeSteps) Wait() error {
	return w.catacomb.Wait()
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

func (w *upgradeSteps) wrenchKey() string {
	return wrenchKey(w.agent.CurrentConfig())
}

func wrenchKey(agentConfig agent.Config) string {
	return agentConfig.Tag().Kind() + "-agent"
}

func (w *upgradeSteps) run() error {
	if wrench.IsActive(w.wrenchKey(), "fail-upgrade-start") {
		return nil // Make the worker stop
	}

	if w.upgradeCompleteLock.IsUnlocked() {
		// Our work is already done (we're probably being restarted
		// because the API connection has gone down), so do nothing.
		return nil
	}

	w.fromVersion = w.agent.CurrentConfig().UpgradedToVersion()
	w.toVersion = jujuversion.Current
	if w.fromVersion == w.toVersion {
		w.logger.Infof("upgrade to %v already completed.", w.toVersion)
		w.upgradeCompleteLock.Unlock()
		return nil
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	if err := w.runUpgrades(ctx); err != nil {
		// Only return an error from the worker if the connection to
		// state went away (possible mongo primary change). Returning
		// an error when the connection is lost will cause the agent
		// to restart.
		//
		// For other errors, the error is not returned because we want
		// the agent to stay running in an error state waiting
		// for user intervention.
		if isAPILostDuringUpgrade(err) {
			return err
		}
		w.reportUpgradeFailure(err, false)
	}

	// Upgrade succeeded - signal that the upgrade is complete.
	w.logger.Infof("upgrade to %v completed successfully.", w.toVersion)
	_ = w.entity.SetStatus(status.Started, "", nil)
	w.upgradeCompleteLock.Unlock()
	return nil
}

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func (w *upgradeSteps) runUpgrades(ctx context.Context) error {
	// Every upgrade needs to prepare the environment for the upgrade.
	w.logger.Infof("checking that upgrade can proceed")
	if err := w.preUpgradeSteps(w.agent.CurrentConfig(), w.isController); err != nil {
		return errors.Annotatef(err, "%s cannot be upgraded", names.ReadableString(w.tag))
	}

	// Handle the easy case first. All non-controller agents can just
	// run the upgrade steps and then return.
	if !w.isController {
		w.logger.Infof("running upgrade steps for %q", w.tag)
		if err := w.agent.ChangeConfig(w.runUpgradeSteps(ctx)); err != nil {
			return errors.Annotatef(err, "failed to run upgrade steps")
		}
	}

	// This is the controller case. We need to ensure that all other
	// controllers are ready to run upgrade steps before we proceed.
	upgradeUUID, err := w.upgradeService.ActiveUpgrade(ctx)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// If there isn't an active upgrade, there isn't anything we can do
			// other than bouncing the worker to see if it can pick it up
			// next time.
			w.logger.Errorf("no active upgrade located")
			return dependency.ErrBounce
		}
		return errors.Trace(err)
	}

	// We should be already in the db completed state. Verify that.
	info, err := w.upgradeService.UpgradeInfo(ctx, upgradeUUID)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// We can't locate the upgrade info, even though we located the
			// active upgrade. This is a bad state to be in, so we'll bounce
			// the worker to see if it can pick it up next time.
			w.logger.Errorf("upgrade %q not found", upgradeUUID)
			return dependency.ErrBounce
		}
		return errors.Trace(err)
	}

	// We're not in the right state, so we can't proceed.
	if info.State != upgrade.DBCompleted {
		w.logger.Errorf("upgrade %q is not in the db completed state %q", upgradeUUID, info.State.String())
		return ErrUpgradeStepsInvalidState
	}

	completedWatcher, err := w.upgradeService.WatchForUpgradeState(ctx, upgradeUUID, upgrade.StepsCompleted)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.addWatcher(ctx, completedWatcher); err != nil {
		return errors.Trace(err)
	}

	failedWatcher, err := w.upgradeService.WatchForUpgradeState(ctx, upgradeUUID, upgrade.Error)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.addWatcher(ctx, failedWatcher); err != nil {
		return errors.Trace(err)
	}

	// Run the upgrade steps in a goroutine so that we can watch for
	// completion or failure.
	go func() {
		w.logger.Infof("running upgrade steps for %q", w.tag)
		if err := w.agent.ChangeConfig(w.runUpgradeSteps(ctx)); err != nil {
			w.logger.Errorf("failed to run upgrade steps: %v", err)
		}
		if err := w.upgradeService.SetControllerDone(ctx, upgradeUUID, w.tag.Id()); err == nil {
			w.logger.Infof("upgrade steps completed for %q", w.tag)
			return
		}

		// TODO (stickupkid): Set upgrade failed and bounce.
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-completedWatcher.Changes():
			return nil

		case <-failedWatcher.Changes():
			// Other controllers have encountered an error, so we can't
			// proceed.
			w.logger.Errorf("upgrade failed")
			return dependency.ErrBounce

		case <-w.clock.After(defaultUpgradeTimeout):
			// TODO (stickupkid): Set upgrade failed and bounce.
			return dependency.ErrBounce
		}
	}
}

// runUpgradeSteps runs the required upgrade steps for the agent,
// retrying on failure. The agent's UpgradedToVersion is set
// once the upgrade is complete.
//
// This function conforms to the agent.ConfigMutator type and is
// designed to be called via an agent's ChangeConfig method.
func (w *upgradeSteps) runUpgradeSteps(ctx context.Context) func(agentConfig agent.ConfigSetter) error {
	return func(agentConfig agent.ConfigSetter) error {
		if err := w.entity.SetStatus(status.Started, fmt.Sprintf("upgrading to %v", w.toVersion), nil); err != nil {
			return errors.Trace(err)
		}

		context := upgrades.NewContext(agentConfig, w.apiCaller)
		w.logger.Infof("starting upgrade from %v to %v for %q", w.fromVersion, w.toVersion, w.tag)

		targets := upgradeTargets(w.isController)

		retryStrategy := retry.CallArgs{
			Clock:    clock.WallClock,
			Delay:    w.retryDelay,
			Attempts: w.retryAttempts,
			IsFatalError: func(err error) bool {
				// Abort if API connection has gone away!
				breakable, ok := w.apiCaller.(agenterrors.Breakable)
				if !ok {
					return false
				}
				return agenterrors.ConnectionIsDead(w.logger, breakable)
			},
			NotifyFunc: func(lastErr error, attempt int) {
				if attempt > 0 && attempt < w.retryAttempts {
					w.reportUpgradeFailure(lastErr, true)
				}
			},
			Func: func() error {
				return w.performUpgradeSteps(w.fromVersion, targets, context)
			},
			Stop: ctx.Done(),
		}

		err := retry.Call(retryStrategy)
		if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
			return retry.LastError(err)
		}
		if err != nil {
			return &apiLostDuringUpgrade{err: err}
		}

		agentConfig.SetUpgradedToVersion(w.toVersion)
		return nil
	}
}

func (w *upgradeSteps) reportUpgradeFailure(err error, willRetry bool) {
	retryText := "will retry"
	if !willRetry {
		retryText = "giving up"
	}
	w.logger.Errorf("upgrade from %v to %v for %q failed (%s): %v",
		w.fromVersion, w.toVersion, w.tag, retryText, err)
	_ = w.entity.SetStatus(status.Error,
		fmt.Sprintf("upgrade to %v failed (%s): %v", w.toVersion, retryText, err), nil)
}

func (w *upgradeSteps) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *upgradeSteps) addWatcher(ctx context.Context, watcher eventsource.Watcher[struct{}]) error {
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	// Consume the initial events from the watchers. The notify watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent[struct{}](ctx, watcher); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// upgradeTargets determines the upgrade targets corresponding to the
// role of an agent. This determines the upgrade steps
// which will run during an upgrade.
func upgradeTargets(isController bool) []upgrades.Target {
	var targets []upgrades.Target
	if isController {
		targets = []upgrades.Target{upgrades.Controller}
	}
	return append(targets, upgrades.HostMachine)
}
