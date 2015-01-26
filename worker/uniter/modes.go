// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
	ucharm "github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
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
		return ModeInstalling(u, opState.CharmURL)
	} else if opState.Kind == operation.Upgrade {
		logger.Infof("resuming charm upgrade")
		return ModeUpgrading(opState.CharmURL), nil
	}

	// If we got this far, we should have an installed charm,
	// so initialize the metrics collector according to what's
	// currently deployed.
	if err := u.initializeMetricsCollector(); err != nil {
		return nil, err
	}

	switch opState.Kind {
	case operation.Continue:
		logger.Infof("continuing after %q hook", opState.Hook.Kind)
		switch opState.Hook.Kind {
		case hooks.Stop:
			return ModeTerminating, nil
		case hooks.UpgradeCharm:
			return ModeConfigChanged, nil
		case hooks.ConfigChanged:
			if !opState.Started {
				return ModeStarting, nil
			}
		}
		if !u.ranConfigChanged {
			return ModeConfigChanged, nil
		}
		return ModeAbide, nil
	case operation.RunHook:
		switch opState.Step {
		case operation.Pending:
			logger.Infof("awaiting error resolution for %q hook", opState.Hook.Kind)
			return ModeHookError, nil
		case operation.Queued:
			logger.Infof("found queued %q hook", opState.Hook.Kind)
			err = u.runHook(*opState.Hook)
		case operation.Done:
			logger.Infof("committing %q hook", opState.Hook.Kind)
			err = u.skipHook(*opState.Hook)
		}
		if err != nil {
			return nil, err
		}
		return ModeContinue, nil
	case operation.RunAction:
		// TODO(fwereade): we *should* handle interrupted actions, and make sure
		// they're marked as failed, but that's not for now.
		logger.Infof("found incomplete action %q; ignoring", opState.ActionId)
		logger.Infof("recommitting prior %q hook", opState.Hook.Kind)
		if err := u.skipHook(*opState.Hook); err != nil {
			return nil, err
		}
		return ModeContinue, nil
	}
	return nil, errors.Errorf("unhandled uniter operation %q", opState.Kind)
}

// ModeInstalling is responsible for the initial charm deployment.
func ModeInstalling(u *Uniter, curl *charm.URL) (next Mode, err error) {
	// First up, set the unit status to Installing.
	if err = u.unit.SetStatus(params.StatusInstalling, "", nil); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("ModeInstalling %s", curl)
	return func(u *Uniter) (next Mode, err error) {
		defer modeContext(name, &err)()
		if err = u.deploy(curl, operation.Install); err != nil {
			return nil, err
		}
		return ModeContinue, nil
	}, nil
}

// ModeUpgrading is responsible for upgrading the charm.
func ModeUpgrading(curl *charm.URL) Mode {
	name := fmt.Sprintf("ModeUpgrading %s", curl)
	return func(u *Uniter) (next Mode, err error) {
		defer modeContext(name, &err)()
		err = u.deploy(curl, operation.Upgrade)
		if errors.Cause(err) == ucharm.ErrConflict {
			return ModeConflicted(curl), nil
		} else if err != nil {
			return nil, err
		}
		return ModeContinue, nil
	}
}

// ModeConfigChanged runs the "config-changed" hook.
func ModeConfigChanged(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeConfigChanged", &err)()
	if !u.operationState().Started {
		if err = u.unit.SetStatus(params.StatusInstalling, "", nil); err != nil {
			return nil, err
		}
	}
	u.f.DiscardConfigEvent()
	err = u.runHook(hook.Info{Kind: hooks.ConfigChanged})
	if err != nil {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeStarting runs the "start" hook.
func ModeStarting(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeStarting", &err)()
	err = u.runHook(hook.Info{Kind: hooks.Start})
	if err != nil {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeStopping runs the "stop" hook.
func ModeStopping(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeStopping", &err)()
	err = u.runHook(hook.Info{Kind: hooks.Stop})
	if err != nil {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeTerminating marks the unit dead and returns ErrTerminateAgent.
func ModeTerminating(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeTerminating", &err)()
	if err = u.unit.SetStatus(params.StatusStopping, "", nil); err != nil {
		return nil, err
	}
	w, err := u.unit.Watch()
	if err != nil {
		return nil, err
	}
	defer watcher.Stop(w, &u.tomb)
	for {
		hi := hook.Info{}
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case info := <-u.f.ActionEvents():
			hi = hook.Info{Kind: info.Kind, ActionId: info.ActionId}
		case _, ok := <-w.Changes():
			if !ok {
				return nil, watcher.EnsureErr(w)
			}
			if err := u.unit.Refresh(); err != nil {
				return nil, err
			}
			if hasSubs, err := u.unit.HasSubordinates(); err != nil {
				return nil, err
			} else if hasSubs {
				continue
			}
			// The unit is known to be Dying; so if it didn't have subordinates
			// just above, it can't acquire new ones before this call.
			if err := u.unit.EnsureDead(); err != nil {
				return nil, err
			}
			return nil, worker.ErrTerminateAgent
		}
		if err := u.runHook(hi); err != nil {
			return nil, err
		}
	}
}

// ModeAbide is the Uniter's usual steady state. It watches for and responds to:
// * service configuration changes
// * charm upgrade requests
// * relation changes
// * storage changes
// * unit death
func ModeAbide(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeAbide", &err)()
	stopHooks := func(kind string, stop func() error) {
		if e := stop(); e != nil {
			if err == nil {
				err = e
			} else {
				logger.Errorf("error while stopping %s hooks: %v", kind, e)
			}
		}
	}

	opState := u.operationState()
	if opState.Kind != operation.Continue {
		return nil, errors.Errorf("insane uniter state: %#v", opState)
	}
	if err := u.fixDeployer(); err != nil {
		return nil, err
	}
	if err = u.unit.SetStatus(params.StatusActive, "", nil); err != nil {
		return nil, err
	}
	u.f.WantUpgradeEvent(false)
	u.relations.StartHooks()
	defer stopHooks("relation", u.relations.StopHooks)
	u.storage.StartHooks()
	defer stopHooks("storage", u.storage.StopHooks)

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
		hi := hook.Info{}
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case <-u.f.UnitDying():
			return modeAbideDyingLoop(u)
		case <-u.f.MeterStatusEvents():
			hi = hook.Info{Kind: hooks.MeterStatusChanged}
		case <-u.f.ConfigEvents():
			hi = hook.Info{Kind: hooks.ConfigChanged}
		case info := <-u.f.ActionEvents():
			hi = hook.Info{Kind: info.Kind, ActionId: info.ActionId}
		case hi = <-u.relations.Hooks():
		case <-collectMetricsSignal:
			hi = hook.Info{Kind: hooks.CollectMetrics}
		case ids := <-u.f.RelationsEvents():
			if err := u.relations.Update(ids); err != nil {
				return nil, err
			}
			continue
		case curl := <-u.f.UpgradeEvents():
			return ModeUpgrading(curl), nil
		case ids := <-u.f.StorageEvents():
			if err := u.storage.Update(ids); err != nil {
				return nil, err
			}
			continue
		case hi = <-u.storage.Hooks():
		}
		if err := u.runHook(hi); err != nil {
			return nil, err
		}
	}
}

// modeAbideDyingLoop handles the proper termination of all subordinates,
// relations and storage instances in response to a Dying unit.
func modeAbideDyingLoop(u *Uniter) (next Mode, err error) {
	if err := u.unit.Refresh(); err != nil {
		return nil, err
	}
	if err = u.unit.DestroyAllSubordinates(); err != nil {
		return nil, err
	}
	if err := u.relations.SetDying(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := u.storage.SetDying(); err != nil {
		return nil, errors.Trace(err)
	}
	for {
		if len(u.relations.GetInfo()) == 0 {
			return ModeStopping, nil
		}
		hi := hook.Info{}
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case <-u.f.ConfigEvents():
			hi = hook.Info{Kind: hooks.ConfigChanged}
		case info := <-u.f.ActionEvents():
			hi = hook.Info{Kind: info.Kind, ActionId: info.ActionId}
		case hi = <-u.relations.Hooks():
		case hi = <-u.storage.Hooks():
		}
		if err := u.runHook(hi); err != nil {
			return nil, err
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
	if err = u.unit.SetStatus(params.StatusError, statusMessage, statusData); err != nil {
		return nil, err
	}
	u.f.WantResolvedEvent()
	u.f.WantUpgradeEvent(true)
	for {
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case rm := <-u.f.ResolvedEvents():
			switch rm {
			case params.ResolvedRetryHooks:
				err = u.runHook(hookInfo)
			case params.ResolvedNoHooks:
				err = u.skipHook(hookInfo)
			default:
				return nil, errors.Errorf("unknown resolved mode %q", rm)
			}
			if e := u.f.ClearResolved(); e != nil {
				return nil, e
			}
			if errors.Cause(err) == operation.ErrHookFailed {
				continue
			} else if err != nil {
				return nil, err
			}
			return ModeContinue, nil
		case curl := <-u.f.UpgradeEvents():
			return ModeUpgrading(curl), nil
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
		if err = u.unit.SetStatus(params.StatusError, "upgrade failed", nil); err != nil {
			return nil, err
		}
		u.f.WantResolvedEvent()
		u.f.WantUpgradeEvent(true)
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case curl = <-u.f.UpgradeEvents():
			if err := u.deployer.NotifyRevert(); err != nil {
				return nil, err
			}
			// Now the git dir (if it is one) has been reverted, it's safe to
			// use a manifest deployer to deploy the new charm.
			if err := u.fixDeployer(); err != nil {
				return nil, err
			}
		case <-u.f.ResolvedEvents():
			err = u.deployer.NotifyResolved()
			if e := u.f.ClearResolved(); e != nil {
				return nil, e
			}
			if err != nil {
				return nil, err
			}
			// We don't fixDeployer at this stage, because we have *no idea*
			// what (if anything) the user has done to the charm dir before
			// setting resolved. But the balance of probability is that the
			// dir is filled with git droppings, that will be considered user
			// files and hang around forever, so in this case we wait for the
			// upgrade to complete and fixDeployer in ModeAbide.
		}
		return ModeUpgrading(curl), nil
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
