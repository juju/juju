// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

// setAgentStatus sets the unit's status if it has changed since last time this method was called.
func setAgentStatus(u *Uniter, status params.Status, info string, data map[string]interface{}) error {
	u.setStatusMutex.Lock()
	defer u.setStatusMutex.Unlock()
	if u.lastReportedStatus == status && u.lastReportedMessage == info {
		return nil
	}
	u.lastReportedStatus = status
	u.lastReportedMessage = info
	logger.Debugf("[AGENT-STATUS] %s: %s", status, info)
	return u.unit.SetAgentStatus(status, info, data)
}

// reportAgentError reports if there was an error performing an agent operation.
func reportAgentError(u *Uniter, userMessage string, err error) {
	// If a non-nil error is reported (e.g. due to an operation failing),
	// set the agent status to Failed.
	if err == nil {
		return
	}
	err2 := setAgentStatus(u, params.StatusFailed, userMessage, nil)
	if err2 != nil {
		logger.Errorf("updating agent status: %v", err2)
	}
}

// Mode defines the signature of the functions that implement the possible
// states of a running Uniter.
type Mode func(u *Uniter) (Mode, error)

// ModeContinue determines what action to take based on persistent uniter state.
func ModeContinue(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeContinue", &err)()
	opState := u.operationState()

	// Resume interrupted deployment operations.
	if opState.Kind == operation.Install {
		logger.Infof("resuming charm install")
		return ModeInstalling(opState.CharmURL)
	} else if opState.Kind == operation.Upgrade {
		logger.Infof("resuming charm upgrade")
		return ModeUpgrading(opState.CharmURL), nil
	}

	// If we got this far, we should have an installed charm,
	// so initialize the metrics timers according to what's
	// currently deployed.
	if err := u.initializeMetricsTimers(); err != nil {
		return nil, errors.Trace(err)
	}

	// Check for any leadership change, and enact it if possible.
	logger.Infof("checking leadership status")
	// If we've already accepted leadership, we don't need to do it again.
	canAcceptLeader := !opState.Leader
	select {
	// If the unit's shutting down, we shouldn't accept it.
	case <-u.f.UnitDying():
		canAcceptLeader = false
	default:
		// If we're in an unexpected mode (eg pending hook) we shouldn't try either.
		if opState.Kind != operation.Continue {
			canAcceptLeader = false
		}
	}

	// NOTE: the Wait() looks scary, but a ClaimLeadership ticket should always
	// complete quickly; worst-case is API latency time, but it's designed that
	// it should be vanishingly rare to hit that code path.
	isLeader := u.leadershipTracker.ClaimLeader().Wait()
	var creator creator
	switch {
	case isLeader && canAcceptLeader:
		creator = newAcceptLeadershipOp()
	case opState.Leader && !isLeader:
		creator = newResignLeadershipOp()
	}
	if creator != nil {
		return continueAfter(u, creator)
	}
	logger.Infof("leadership status is up-to-date")

	switch opState.Kind {
	case operation.RunAction:
		// TODO(fwereade): we *should* handle interrupted actions, and make sure
		// they're marked as failed, but that's not for now.
		if opState.Hook != nil {
			logger.Infof("found incomplete action %q; ignoring", opState.ActionId)
			logger.Infof("recommitting prior %q hook", opState.Hook.Kind)
			creator = newSkipHookOp(*opState.Hook)
		} else {
			logger.Infof("%q hook is nil", operation.RunAction)
		}
	case operation.RunHook:
		switch opState.Step {
		case operation.Pending:
			logger.Infof("awaiting error resolution for %q hook", opState.Hook.Kind)
			return ModeHookError, nil
		case operation.Queued:
			logger.Infof("found queued %q hook", opState.Hook.Kind)
			// Ensure storage-attached hooks are run before install
			// or upgrade hooks.
			switch opState.Hook.Kind {
			case hooks.UpgradeCharm:
				// Force a refresh of all storage attachments,
				// so we find out about new ones introduced
				// by the charm upgrade.
				if err := u.storage.Refresh(); err != nil {
					return nil, errors.Trace(err)
				}
				fallthrough
			case hooks.Install:
				if err := waitStorage(u); err != nil {
					return nil, errors.Trace(err)
				}
			}
			creator = newRunHookOp(*opState.Hook)
		case operation.Done:
			logger.Infof("committing %q hook", opState.Hook.Kind)
			creator = newSkipHookOp(*opState.Hook)
		}
	case operation.Continue:
		if opState.Stopped {
			logger.Infof("opState.Stopped == true; transition to ModeTerminating")
			return ModeTerminating, nil
		}
		logger.Infof("no operations in progress; waiting for changes")
		return ModeAbide, nil
	default:
		return nil, errors.Errorf("unknown operation kind %v", opState.Kind)
	}
	return continueAfter(u, creator)
}

// ModeInstalling is responsible for the initial charm deployment. If an install
// operation were to set an appropriate status, it shouldn't be necessary; but see
// ModeUpgrading for discussion relevant to both.
func ModeInstalling(curl *charm.URL) (next Mode, err error) {
	name := fmt.Sprintf("ModeInstalling %s", curl)
	return func(u *Uniter) (next Mode, err error) {
		defer modeContext(name, &err)()
		return continueAfter(u, newInstallOp(curl))
	}, nil
}

// ModeUpgrading is responsible for upgrading the charm. It shouldn't really
// need to be a mode at all -- it's just running a single operation -- but
// it's not safe to call it inside arbitrary other modes, because failing to
// pass through ModeContinue on the way out could cause a queued hook to be
// accidentally skipped.
func ModeUpgrading(curl *charm.URL) Mode {
	name := fmt.Sprintf("ModeUpgrading %s", curl)
	return func(u *Uniter) (next Mode, err error) {
		defer modeContext(name, &err)()
		return continueAfter(u, newUpgradeOp(curl))
	}
}

// ModeTerminating marks the unit dead and returns ErrTerminateAgent.
func ModeTerminating(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeTerminating", &err)()
	w, err := u.unit.Watch()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer watcher.Stop(w, &u.tomb)

	// Upon unit termination we attempt to send any leftover metrics one last time. If we fail, there is nothing
	// else we can do but log the error.
	sendErr := u.runOperation(newSendMetricsOp())
	if sendErr != nil {
		logger.Warningf("failed to send metrics: %v", sendErr)
	}

	for {
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case actionId := <-u.f.ActionEvents():
			creator := newActionOp(actionId)
			if err := u.runOperation(creator); err != nil {
				return nil, errors.Trace(err)
			}
		case _, ok := <-w.Changes():
			if !ok {
				return nil, watcher.EnsureErr(w)
			}
			if err := u.unit.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			if hasSubs, err := u.unit.HasSubordinates(); err != nil {
				return nil, errors.Trace(err)
			} else if hasSubs {
				continue
			}
			// The unit is known to be Dying; so if it didn't have subordinates
			// just above, it can't acquire new ones before this call.
			if err := u.unit.EnsureDead(); err != nil {
				return nil, errors.Trace(err)
			}
			return nil, worker.ErrTerminateAgent
		}
	}
}

// ModeAbide is the Uniter's usual steady state. It watches for and responds to:
// * service configuration changes
// * charm upgrade requests
// * relation changes
// * unit death
// * acquisition or loss of service leadership
func ModeAbide(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeAbide", &err)()
	opState := u.operationState()
	if opState.Kind != operation.Continue {
		return nil, errors.Errorf("insane uniter state: %#v", opState)
	}
	if err := u.deployer.Fix(); err != nil {
		return nil, errors.Trace(err)
	}

	if !opState.Leader && !u.ranLeaderSettingsChanged {
		creator := newSimpleRunHookOp(hook.LeaderSettingsChanged)
		if err := u.runOperation(creator); err != nil {
			return nil, errors.Trace(err)
		}
	}

	if !u.ranConfigChanged {
		return continueAfter(u, newSimpleRunHookOp(hooks.ConfigChanged))
	}
	if !opState.Started {
		return continueAfter(u, newSimpleRunHookOp(hooks.Start))
	}
	u.f.WantUpgradeEvent(false)
	u.relations.StartHooks()
	defer func() {
		if e := u.relations.StopHooks(); e != nil {
			if err == nil {
				err = e
			} else {
				logger.Errorf("error while stopping hooks: %v", e)
			}
		}
	}()

	select {
	case <-u.f.UnitDying():
		return modeAbideDyingLoop(u)
	default:
	}
	return modeAbideAliveLoop(u)
}

// idleWaitTime is the time after which, if there are no uniter events,
// the agent state becomes idle.
var idleWaitTime = 2 * time.Second

// modeAbideAliveLoop handles all state changes for ModeAbide when the unit
// is in an Alive state.
func modeAbideAliveLoop(u *Uniter) (Mode, error) {
	var leaderElected, leaderDeposed <-chan struct{}
	for {
		// We expect one or none of these vars to be non-nil; and if none
		// are, we set the one that should trigger when our leadership state
		// differs from what we have recorded locally.
		if leaderElected == nil && leaderDeposed == nil {
			if u.operationState().Leader {
				logger.Infof("waiting to lose leadership")
				leaderDeposed = u.leadershipTracker.WaitMinion().Ready()
			} else {
				logger.Infof("waiting to gain leadership")
				leaderElected = u.leadershipTracker.WaitLeader().Ready()
			}
		}

		// collect-metrics hook
		lastCollectMetrics := time.Unix(u.operationState().CollectMetricsTime, 0)
		collectMetricsSignal := u.collectMetricsAt(
			time.Now(), lastCollectMetrics, metricsPollInterval,
		)

		lastSentMetrics := time.Unix(u.operationState().SendMetricsTime, 0)
		sendMetricsSignal := u.sendMetricsAt(
			time.Now(), lastSentMetrics, metricsSendInterval,
		)

		// update-status hook
		lastUpdateStatus := time.Unix(u.operationState().UpdateStatusTime, 0)
		updateStatusSignal := u.updateStatusAt(
			time.Now(), lastUpdateStatus, statusPollInterval,
		)

		var creator creator
		select {
		case <-time.After(idleWaitTime):
			if err := setAgentStatus(u, params.StatusIdle, "", nil); err != nil {
				return nil, errors.Trace(err)
			}
			continue
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case <-u.f.UnitDying():
			return modeAbideDyingLoop(u)
		case curl := <-u.f.UpgradeEvents():
			return ModeUpgrading(curl), nil
		case ids := <-u.f.RelationsEvents():
			creator = newUpdateRelationsOp(ids)
		case actionId := <-u.f.ActionEvents():
			creator = newActionOp(actionId)
		case tags := <-u.f.StorageEvents():
			creator = newUpdateStorageOp(tags)
		case <-u.f.ConfigEvents():
			creator = newSimpleRunHookOp(hooks.ConfigChanged)
		case <-u.f.MeterStatusEvents():
			creator = newSimpleRunHookOp(hooks.MeterStatusChanged)
		case <-collectMetricsSignal:
			creator = newSimpleRunHookOp(hooks.CollectMetrics)
		case <-sendMetricsSignal:
			creator = newSendMetricsOp()
		case <-updateStatusSignal:
			creator = newSimpleRunHookOp(hooks.UpdateStatus)
		case hookInfo := <-u.relations.Hooks():
			creator = newRunHookOp(hookInfo)
		case hookInfo := <-u.storage.Hooks():
			creator = newRunHookOp(hookInfo)
		case <-leaderElected:
			// This operation queues a hook, better to let ModeContinue pick up
			// after it than to duplicate queued-hook handling here.
			return continueAfter(u, newAcceptLeadershipOp())
		case <-leaderDeposed:
			leaderDeposed = nil
			creator = newResignLeadershipOp()
		case <-u.f.LeaderSettingsEvents():
			creator = newSimpleRunHookOp(hook.LeaderSettingsChanged)
		}
		if err := u.runOperation(creator); err != nil {
			return nil, errors.Trace(err)
		}
	}
}

// modeAbideDyingLoop handles the proper termination of all relations in
// response to a Dying unit.
func modeAbideDyingLoop(u *Uniter) (next Mode, err error) {
	if err := u.unit.Refresh(); err != nil {
		return nil, errors.Trace(err)
	}
	if err = u.unit.DestroyAllSubordinates(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := u.relations.SetDying(); err != nil {
		return nil, errors.Trace(err)
	}
	if u.operationState().Leader {
		if err := u.runOperation(newResignLeadershipOp()); err != nil {
			return nil, errors.Trace(err)
		}
		// TODO(fwereade): we ought to inform the tracker that we're shutting down
		// (and no longer wish to continue renewing our lease) so that the tracker
		// can then report minionhood at all times, and thus prevent the is-leader
		// and leader-set hook tools from acting in a correct but misleading way
		// (ie continuing to act as though leader after leader-deposed has run).
	}
	if err := u.storage.SetDying(); err != nil {
		return nil, errors.Trace(err)
	}
	for {
		if len(u.relations.GetInfo()) == 0 && u.storage.Empty() {
			return continueAfter(u, newSimpleRunHookOp(hooks.Stop))
		}
		var creator creator
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case actionId := <-u.f.ActionEvents():
			creator = newActionOp(actionId)
		case <-u.f.ConfigEvents():
			creator = newSimpleRunHookOp(hooks.ConfigChanged)
		case <-u.f.LeaderSettingsEvents():
			creator = newSimpleRunHookOp(hook.LeaderSettingsChanged)
		case hookInfo := <-u.relations.Hooks():
			creator = newRunHookOp(hookInfo)
		case hookInfo := <-u.storage.Hooks():
			creator = newRunHookOp(hookInfo)
		}
		if err := u.runOperation(creator); err != nil {
			return nil, errors.Trace(err)
		}
	}
}

// waitStorage waits until all storage attachments are provisioned
// and their hooks processed.
func waitStorage(u *Uniter) error {
	if u.storage.Pending() == 0 {
		return nil
	}
	logger.Infof("waiting for storage attachments")
	for u.storage.Pending() > 0 {
		var creator creator
		select {
		case <-u.tomb.Dying():
			return tomb.ErrDying
		case <-u.f.UnitDying():
			// Unit is shutting down; no need to handle any
			// more storage-attached hooks. We will process
			// required storage-detaching hooks in ModeAbideDying.
			return nil
		case tags := <-u.f.StorageEvents():
			creator = newUpdateStorageOp(tags)
		case hookInfo := <-u.storage.Hooks():
			creator = newRunHookOp(hookInfo)
		}
		if err := u.runOperation(creator); err != nil {
			return errors.Trace(err)
		}
	}
	logger.Infof("storage attachments ready")
	return nil
}

// ModeHookError is responsible for watching and responding to:
// * user resolution of hook errors
// * forced charm upgrade requests
// * loss of service leadership
func ModeHookError(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeHookError", &err)()
	opState := u.operationState()
	if opState.Kind != operation.RunHook || opState.Step != operation.Pending {
		return nil, errors.Errorf("insane uniter state: %#v", u.operationState())
	}

	// Create error information for status.
	hookInfo := *opState.Hook
	hookName := string(hookInfo.Kind)
	statusData := map[string]interface{}{}
	if hookInfo.Kind.IsRelation() {
		statusData["relation-id"] = hookInfo.RelationId
		if hookInfo.RemoteUnit != "" {
			statusData["remote-unit"] = hookInfo.RemoteUnit
		}
		relationName, err := u.relations.Name(hookInfo.RelationId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		hookName = fmt.Sprintf("%s-%s", relationName, hookInfo.Kind)
	}
	statusData["hook"] = hookName
	statusMessage := fmt.Sprintf("hook failed: %q", hookName)

	// Run the select loop.
	u.f.WantResolvedEvent()
	u.f.WantUpgradeEvent(true)
	var leaderDeposed <-chan struct{}
	if opState.Leader {
		leaderDeposed = u.leadershipTracker.WaitMinion().Ready()
	}
	for {
		// The spec says we should set the workload status to Error, but that's crazy talk.
		// It's the agent itself that should be in Error state. So we'll ensure the model is
		// correct and translate before the user sees the data.
		// ie a charm hook error results in agent error status, but is presented as a workload error.
		if err = setAgentStatus(u, params.StatusError, statusMessage, statusData); err != nil {
			return nil, errors.Trace(err)
		}
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case curl := <-u.f.UpgradeEvents():
			return ModeUpgrading(curl), nil
		case rm := <-u.f.ResolvedEvents():
			var creator creator
			switch rm {
			case params.ResolvedRetryHooks:
				creator = newRetryHookOp(hookInfo)
			case params.ResolvedNoHooks:
				creator = newSkipHookOp(hookInfo)
			default:
				return nil, errors.Errorf("unknown resolved mode %q", rm)
			}
			err := u.runOperation(creator)
			if errors.Cause(err) == operation.ErrHookFailed {
				continue
			} else if err != nil {
				return nil, errors.Trace(err)
			}
			return ModeContinue, nil
		case actionId := <-u.f.ActionEvents():
			if err := u.runOperation(newActionOp(actionId)); err != nil {
				return nil, errors.Trace(err)
			}
		case <-leaderDeposed:
			// This should trigger at most once -- we can't reaccept leadership while
			// in an error state.
			leaderDeposed = nil
			if err := u.runOperation(newResignLeadershipOp()); err != nil {
				return nil, errors.Trace(err)
			}
		}
	}
}

// ModeConflicted is responsible for watching and responding to:
// * user resolution of charm upgrade conflicts
// * forced charm upgrade requests
func ModeConflicted(curl *charm.URL) Mode {
	return func(u *Uniter) (next Mode, err error) {
		defer modeContext("ModeConflicted", &err)()
		// TODO(mue) Add helpful data here too in later CL.
		// The spec says we should set the workload status to Error, but that's crazy talk.
		// It's the agent itself that should be in Error state. So we'll ensure the model is
		// correct and translate before the user sees the data.
		// ie a charm upgrade error results in agent error status, but is presented as a workload error.
		if err := setAgentStatus(u, params.StatusError, "upgrade failed", nil); err != nil {
			return nil, errors.Trace(err)
		}
		u.f.WantResolvedEvent()
		u.f.WantUpgradeEvent(true)
		var creator creator
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case curl = <-u.f.UpgradeEvents():
			creator = newRevertUpgradeOp(curl)
		case <-u.f.ResolvedEvents():
			creator = newResolvedUpgradeOp(curl)
		}
		return continueAfter(u, creator)
	}
}

// modeContext returns a function that implements logging and common error
// manipulation for Mode funcs.
func modeContext(name string, err *error) func() {
	logger.Infof("%s starting", name)
	return func() {
		logger.Infof("%s exiting", name)
		*err = errors.Annotatef(*err, name)
	}
}

// continueAfter is commonly used at the end of a Mode func to execute the
// operation returned by creator and return ModeContinue (or any error).
func continueAfter(u *Uniter, creator creator) (Mode, error) {
	if err := u.runOperation(creator); err != nil {
		return nil, errors.Trace(err)
	}
	return ModeContinue, nil
}
