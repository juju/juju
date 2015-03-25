// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5-unstable"
	"gopkg.in/juju/charm.v5-unstable/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/operation"
)

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
	// so initialize the metrics collector according to what's
	// currently deployed.
	if err := u.initializeMetricsCollector(); err != nil {
		return nil, errors.Trace(err)
	}

	var creator creator
	switch opState.Kind {
	case operation.RunAction:
		// TODO(fwereade): we *should* handle interrupted actions, and make sure
		// they're marked as failed, but that's not for now.
		logger.Infof("found incomplete action %q; ignoring", opState.ActionId)
		logger.Infof("recommitting prior %q hook", opState.Hook.Kind)
		creator = newSkipHookOp(*opState.Hook)
	case operation.RunHook:
		switch opState.Step {
		case operation.Pending:
			logger.Infof("awaiting error resolution for %q hook", opState.Hook.Kind)
			return ModeHookError, nil
		case operation.Queued:
			logger.Infof("found queued %q hook", opState.Hook.Kind)
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
		// TODO(fwereade) 2015-01-19
		// This SetUnitStatus call should probably be inside the operation somehow;
		// which in turn implies that the SetUnitStatus call in PrepareHook is
		// also misplaced, and should also be explicitly part of the operation.
		if err = u.unit.SetUnitStatus(params.StatusMaintenance, "", nil); err != nil {
			return nil, errors.Trace(err)
		}
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
	if err = u.unit.SetUnitStatus(params.StatusMaintenance, "", nil); err != nil {
		return nil, errors.Trace(err)
	}
	w, err := u.unit.Watch()
	if err != nil {
		return nil, errors.Trace(err)
	}

	defer watcher.Stop(w, &u.tomb)

	//TODO(perrito666) Should this be a mode?
	defer func() {
		if err != worker.ErrTerminateAgent {
			return
		}
		if err = u.unit.SetUnitStatus(params.StatusTerminated, "", nil); err != nil {
			logger.Errorf("cannot set unit status to %q: %v", params.StatusTerminated, err)
		}
	}()
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
func ModeAbide(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeAbide", &err)()
	opState := u.operationState()
	if opState.Kind != operation.Continue {
		return nil, errors.Errorf("insane uniter state: %#v", opState)
	}
	if err := u.deployer.Fix(); err != nil {
		return nil, errors.Trace(err)
	}
	if !u.ranConfigChanged {
		return continueAfter(u, newSimpleRunHookOp(hooks.ConfigChanged))
	}
	if !opState.Started {
		return continueAfter(u, newSimpleRunHookOp(hooks.Start))
	}
	if err = u.unit.SetUnitStatus(params.StatusActive, "", nil); err != nil {
		return nil, errors.Trace(err)
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

// modeAbideAliveLoop handles all state changes for ModeAbide when the unit
// is in an Alive state.
func modeAbideAliveLoop(u *Uniter) (Mode, error) {
	for {
		lastCollectMetrics := time.Unix(u.operationState().CollectMetricsTime, 0)
		collectMetricsSignal := u.collectMetricsAt(
			time.Now(), lastCollectMetrics, metricsPollInterval,
		)
		var creator creator
		select {
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
	for {
		if len(u.relations.GetInfo()) == 0 {
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
		case hookInfo := <-u.relations.Hooks():
			creator = newRunHookOp(hookInfo)
		}
		if err := u.runOperation(creator); err != nil {
			return nil, errors.Trace(err)
		}
	}
}

// ModeHookError is responsible for watching and responding to:
// * user resolution of hook errors
// * forced charm upgrade requests
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
	u.f.WantResolvedEvent()
	u.f.WantUpgradeEvent(true)
	for {
		if err = u.unit.SetUnitStatus(params.StatusError, statusMessage, statusData); err != nil {
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
		if err = u.unit.SetUnitStatus(params.StatusBlocked, "upgrade failed", nil); err != nil {
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
		logger.Debugf("%s exiting", name)
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
