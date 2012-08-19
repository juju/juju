package uniter

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/worker/uniter/charm"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/tomb"
)

// Mode defines the signature of the functions that implement the possible
// states of a running Uniter.
type Mode func(u *Uniter) (Mode, error)

// ModeInit determines the Mode in which to start a new Uniter.
func ModeInit(u *Uniter) (mode Mode, err error) {
	defer trivial.ErrorContextf(&err, "ModeInit")
	if status, err := u.charm.Read(); err != nil {
		return nil, err
	} else if status == charm.Missing {
		return ModeChangingCharm(charm.Installing), nil
	} else if status != charm.Installed {
		return ModeChangingCharm(status), nil
	}
	return nextMode(u)
}

// nextMode determines the next Mode to run, based purely on hook
// state. Potentially-inconsistent state will be synchronized.
func nextMode(u *Uniter) (Mode, error) {
	hs, err := u.hook.Read()
	if err != nil {
		return nil, err
	} else if hs.Status == hook.StatusStarted {
		return ModeHookError, nil
	} else if hs.Status == hook.StatusSucceeded {
		if err = u.commitHook(hs.Info); err != nil {
			return nil, err
		}
	}
	if hs.Info.Kind == hook.Install {
		return ModeStarting, nil
	}
	return ModeStarted, nil
}

// ModeChangingCharm returns a Mode that will perform a charm change operation
// identified by reason.
func ModeChangingCharm(reason charm.Status) Mode {
	if !reason.IsChange() {
		panic(fmt.Errorf("invalid charm change reason %q", reason))
	}
	return func(u *Uniter) (mode Mode, err error) {
		defer func() {
			// Compress common handling to make logic below clearer.
			if mode == nil && (err == nil || err == errHookFailed) {
				mode, err = nextMode(u)
			}
			if err != nil {
				err = fmt.Errorf("ModeChangingCharm(%q): %s", reason, err)
			}
		}()
		changedCharm, err := u.changeCharm(reason)
		if err != nil {
			return nil, err
		}
		var hi hook.Info
		switch reason {
		case charm.UpgradingForced:
			// Forced upgrades do not run hooks; mark the operation complete.
			return nil, u.syncState(hook.Info{Kind: hook.UpgradeCharm})
		case charm.Upgrading:
			hi.Kind = hook.UpgradeCharm
		case charm.Installing:
			hi.Kind = hook.Install
		default:
			panic(fmt.Errorf("unhandled charm change reason %q", reason))
		}
		if !changedCharm {
			if status, err := u.charm.Read(); err != nil {
				return nil, err
			} else if status == charm.Installed {
				// We didn't need to do anything: mark the operation complete.
				return nil, u.syncState(hi)
			}
			if hs, _ := u.hook.Read(); hs.Info.Kind == hi.Kind {
				if hs.Status == hook.StatusCommitted {
					return nil, fmt.Errorf("inconsistent %q hook status %q", hi.Kind, hs.Status)
				}
				// We'd already started to run the hook; we should be in a
				// different mode. State will be synced when the hook is
				// committed.
				return
			}
		}
		return nil, u.runHook(hi)
	}
}

// ModeStarting is responsible for running the "start" hook.
func ModeStarting(u *Uniter) (mode Mode, err error) {
	defer trivial.ErrorContextf(&err, "ModeStarting")
	if err := u.unit.SetStatus(state.UnitInstalled, ""); err != nil {
		return nil, err
	}
	hi := hook.Info{Kind: hook.Start}
	if err := u.runHook(hi); err != nil && err != errHookFailed {
		return nil, err
	}
	return nextMode(u)
}

// ModeStarted is the Uniter's usual steady state. It watches for and responds to:
// * charm upgrade requests
// * service configuration changes
// * relation changes (not implemented)
// * the death of the managed unit (not implemented)
func ModeStarted(u *Uniter) (mode Mode, err error) {
	defer trivial.ErrorContextf(&err, "ModeStarted")
	if err = u.unit.SetStatus(state.UnitStarted, ""); err != nil {
		return nil, err
	}
	starting := true
	var upgrade *state.NeedsUpgradeWatcher
	var upgrades <-chan state.NeedsUpgrade
	// To guarantee the first hook we run is "config-changed" hook, start
	// off watching only for the initial config change event...
	config := u.service.WatchConfig()
	defer watcher.Stop(config, &u.tomb)
	for {
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case _, ok := <-config.Changes():
			if !ok {
				return nil, watcher.MustErr(config)
			}
			hi := hook.Info{Kind: hook.ConfigChanged}
			if err = u.runHook(hi); err != nil {
				if err == errHookFailed {
					return ModeHookError, nil
				}
				return nil, err
			}
			if starting {
				// ...then, once we've handled that, start other watches.
				starting = false
				upgrade = u.unit.WatchNeedsUpgrade()
				defer watcher.Stop(upgrade, &u.tomb)
				upgrades = upgrade.Changes()
				// TODO: relations
			}
		case ch, ok := <-upgrades:
			if !ok {
				return nil, watcher.MustErr(upgrade)
			}
			if ch.Upgrade {
				if ch.Force {
					return ModeChangingCharm(charm.UpgradingForced), nil
				}
				return ModeChangingCharm(charm.Upgrading), nil
			}
		}
		// TODO: unit death; relations.
	}
	panic("unreachable")
}

// ModeHookError is responsible for watching and responding to:
// * user resolution of hook errors
// * forced charm upgrade requests
// * the death of the managed unit (not implemented)
func ModeHookError(u *Uniter) (mode Mode, err error) {
	defer trivial.ErrorContextf(&err, "ModeHookError")
	hs, err := u.hook.Read()
	if err != nil {
		return nil, err
	}
	if hs.Status != hook.StatusStarted {
		return nil, fmt.Errorf("inconsistent hook status %q", hs.Status)
	}
	msg := fmt.Sprintf("failed to run %q hook", hs.Info.Kind)
	if err = u.unit.SetStatus(state.UnitError, msg); err != nil {
		return nil, err
	}

	// Wait for shutdown, error resolution, or forced charm upgrade.
	resolved := u.unit.WatchResolved()
	defer watcher.Stop(resolved, &u.tomb)
	upgrade := u.unit.WatchNeedsUpgrade()
	defer watcher.Stop(upgrade, &u.tomb)
	for {
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case ch, ok := <-upgrade.Changes():
			if !ok {
				return nil, watcher.MustErr(upgrade)
			}
			if ch.Upgrade && ch.Force {
				return ModeChangingCharm(charm.UpgradingForced), nil
			}
		case rm, ok := <-resolved.Changes():
			if !ok {
				return nil, watcher.MustErr(resolved)
			}
			if rm == state.ResolvedNone {
				continue
			}
			if rm == state.ResolvedRetryHooks {
				err = u.runHook(hs.Info)
			} else {
				err = u.commitHook(hs.Info)
			}
			if err != nil && err != errHookFailed {
				return nil, err
			}
			if err = u.unit.ClearResolved(); err != nil {
				return nil, err
			}
			if err == errHookFailed {
				continue
			}
			return ModeStarted, nil
		}
		// TODO: unit death.
	}
	panic("unreachable")
}
