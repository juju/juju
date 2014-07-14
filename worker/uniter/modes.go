// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stderrors "errors"
	"fmt"

	"github.com/juju/charm"
	"github.com/juju/charm/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
	ucharm "github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
)

// Mode defines the signature of the functions that implement the possible
// states of a running Uniter.
type Mode func(u *Uniter) (Mode, error)

// ModeContinue determines what action to take based on persistent uniter state.
func ModeContinue(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeContinue", &err)()

	// If we haven't yet loaded state, do so.
	if u.s == nil {
		logger.Infof("loading uniter state")
		if u.s, err = u.sf.Read(); err == ErrNoStateFile {
			// When no state exists, start from scratch.
			logger.Infof("charm is not deployed")
			curl, _, err := u.service.CharmURL()
			if err != nil {
				return nil, err
			}
			return ModeInstalling(curl), nil
		} else if err != nil {
			return nil, err
		}
	}

	// Filter out states not related to charm deployment.
	switch u.s.Op {
	case Continue:
		logger.Infof("continuing after %q hook", u.s.Hook.Kind)
		switch u.s.Hook.Kind {
		case hooks.Stop:
			return ModeTerminating, nil
		case hooks.UpgradeCharm:
			return ModeConfigChanged, nil
		case hooks.ConfigChanged:
			if !u.s.Started {
				return ModeStarting, nil
			}
		}
		if !u.ranConfigChanged {
			return ModeConfigChanged, nil
		}
		return ModeAbide, nil
	case RunHook:
		if u.s.OpStep == Queued {
			logger.Infof("found queued %q hook", u.s.Hook.Kind)
			if err = u.runHook(*u.s.Hook); err != nil && err != errHookFailed {
				return nil, err
			}
			return ModeContinue, nil
		}
		if u.s.OpStep == Done {
			logger.Infof("found uncommitted %q hook", u.s.Hook.Kind)
			if err = u.commitHook(*u.s.Hook); err != nil {
				return nil, err
			}
			return ModeContinue, nil
		}
		logger.Infof("awaiting error resolution for %q hook", u.s.Hook.Kind)
		return ModeHookError, nil
	}

	// Resume interrupted deployment operations.
	curl := u.s.CharmURL
	if u.s.Op == Install {
		logger.Infof("resuming charm install")
		return ModeInstalling(curl), nil
	} else if u.s.Op == Upgrade {
		logger.Infof("resuming charm upgrade")
		return ModeUpgrading(curl), nil
	}
	panic(fmt.Errorf("unhandled uniter operation %q", u.s.Op))
}

// ModeInstalling is responsible for the initial charm deployment.
func ModeInstalling(curl *charm.URL) Mode {
	name := fmt.Sprintf("ModeInstalling %s", curl)
	return func(u *Uniter) (next Mode, err error) {
		defer modeContext(name, &err)()
		if err = u.deploy(curl, Install); err != nil {
			return nil, err
		}
		return ModeContinue, nil
	}
}

// ModeUpgrading is responsible for upgrading the charm.
func ModeUpgrading(curl *charm.URL) Mode {
	name := fmt.Sprintf("ModeUpgrading %s", curl)
	return func(u *Uniter) (next Mode, err error) {
		defer modeContext(name, &err)()
		if err = u.deploy(curl, Upgrade); err == ucharm.ErrConflict {
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
	if !u.s.Started {
		if err = u.unit.SetStatus(params.StatusInstalled, "", nil); err != nil {
			return nil, err
		}
	}
	u.f.DiscardConfigEvent()
	if err := u.runHook(hook.Info{Kind: hooks.ConfigChanged}); err == errHookFailed {
		return ModeHookError, nil
	} else if err != nil {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeStarting runs the "start" hook.
func ModeStarting(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeStarting", &err)()
	if err := u.runHook(hook.Info{Kind: hooks.Start}); err == errHookFailed {
		return ModeHookError, nil
	} else if err != nil {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeStopping runs the "stop" hook.
func ModeStopping(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeStopping", &err)()
	if err := u.runHook(hook.Info{Kind: hooks.Stop}); err == errHookFailed {
		return ModeHookError, nil
	} else if err != nil {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeTerminating marks the unit dead and returns ErrTerminateAgent.
func ModeTerminating(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeTerminating", &err)()
	if err = u.unit.SetStatus(params.StatusStopped, "", nil); err != nil {
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
				return nil, watcher.MustErr(w)
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
		if err := u.runHook(hi); err == errHookFailed {
			return ModeHookError, nil
		} else if err != nil {
			return nil, err
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
	if u.s.Op != Continue {
		return nil, fmt.Errorf("insane uniter state: %#v", u.s)
	}
	if err := u.fixDeployer(); err != nil {
		return nil, err
	}
	if err = u.unit.SetStatus(params.StatusStarted, "", nil); err != nil {
		return nil, err
	}
	u.f.WantUpgradeEvent(false)
	for _, r := range u.relationers {
		r.StartHooks()
	}
	defer func() {
		for _, r := range u.relationers {
			if e := r.StopHooks(); e != nil && err == nil {
				err = e
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
		hi := hook.Info{}
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case <-u.f.UnitDying():
			return modeAbideDyingLoop(u)
		case <-u.f.ConfigEvents():
			hi = hook.Info{Kind: hooks.ConfigChanged}
		case info := <-u.f.ActionEvents():
			hi = hook.Info{Kind: info.Kind, ActionId: info.ActionId}
		case hi = <-u.relationHooks:
		case ids := <-u.f.RelationsEvents():
			added, err := u.updateRelations(ids)
			if err != nil {
				return nil, err
			}
			for _, r := range added {
				r.StartHooks()
			}
			continue
		case curl := <-u.f.UpgradeEvents():
			return ModeUpgrading(curl), nil
		}
		if err := u.runHook(hi); err == errHookFailed {
			return ModeHookError, nil
		} else if err != nil {
			return nil, err
		}
	}
}

// modeAbideDyingLoop handles the proper termination of all relations in
// response to a Dying unit.
func modeAbideDyingLoop(u *Uniter) (next Mode, err error) {
	if err := u.unit.Refresh(); err != nil {
		return nil, err
	}
	if err = u.unit.DestroyAllSubordinates(); err != nil {
		return nil, err
	}
	for id, r := range u.relationers {
		if err := r.SetDying(); err != nil {
			return nil, err
		} else if r.IsImplicit() {
			delete(u.relationers, id)
		}
	}
	for {
		if len(u.relationers) == 0 {
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
		case hi = <-u.relationHooks:
		}
		if err = u.runHook(hi); err == errHookFailed {
			return ModeHookError, nil
		} else if err != nil {
			return nil, err
		}
	}
}

// ModeHookError is responsible for watching and responding to:
// * user resolution of hook errors
// * forced charm upgrade requests
func ModeHookError(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeHookError", &err)()
	if u.s.Op != RunHook || u.s.OpStep != Pending {
		return nil, fmt.Errorf("insane uniter state: %#v", u.s)
	}
	msg := fmt.Sprintf("hook failed: %q", u.currentHookName())
	// Create error information for status.
	data := params.StatusData{"hook": u.currentHookName()}
	if u.s.Hook.Kind.IsRelation() {
		data["relation-id"] = u.s.Hook.RelationId
		if u.s.Hook.RemoteUnit != "" {
			data["remote-unit"] = u.s.Hook.RemoteUnit
		}
	}
	if err = u.unit.SetStatus(params.StatusError, msg, data); err != nil {
		return nil, err
	}
	u.f.WantResolvedEvent()
	u.f.WantUpgradeEvent(true)
	for {
		hi := hook.Info{}
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case info := <-u.f.ActionEvents():
			hi = hook.Info{Kind: info.Kind, ActionId: info.ActionId}
		case rm := <-u.f.ResolvedEvents():
			switch rm {
			case params.ResolvedRetryHooks:
				err = u.runHook(*u.s.Hook)
			case params.ResolvedNoHooks:
				err = u.commitHook(*u.s.Hook)
			default:
				return nil, fmt.Errorf("unknown resolved mode %q", rm)
			}
			if e := u.f.ClearResolved(); e != nil {
				return nil, e
			}
			if err == errHookFailed {
				continue
			} else if err != nil {
				return nil, err
			}
			return ModeContinue, nil
		case curl := <-u.f.UpgradeEvents():
			return ModeUpgrading(curl), nil
		}
		if err := u.runHook(hi); err == errHookFailed {
			return ModeHookError, nil
		} else if err != nil {
			return nil, err
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
		switch *err {
		case nil, tomb.ErrDying, worker.ErrTerminateAgent:
		default:
			*err = stderrors.New(name + ": " + (*err).Error())
		}
	}
}
