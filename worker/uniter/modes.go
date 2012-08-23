package uniter

import (
	"fmt"
	"launchpad.net/juju-core/log"
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
	log.Printf("examining charm state...")
	var sch *state.Charm
	st, url, err := u.charm.ReadStatus()
	if err == charm.ErrMissing {
		log.Printf("charm is not installed")
		if sch, _, err = u.service.Charm(); err != nil {
			return nil, err
		}
		return ModeInstalling(sch), nil
	} else if err != nil {
		return nil, err
	}
	if st == charm.Installed {
		log.Printf("charm is installed")
		return nextMode(u)
	} else if sch, err = u.st.Charm(url); err != nil {
		return nil, err
	}
	switch st {
	case charm.Installing:
		log.Printf("resuming charm install")
		return ModeInstalling(sch), nil
	case charm.Upgrading, charm.Conflicted:
		panic("not implemented")
	}
	panic("unreachable")
}

// nextMode determines the next Mode to run, based purely on hook
// state. Potentially-inconsistent state will be synchronized.
func nextMode(u *Uniter) (Mode, error) {
	log.Printf("examining hook state...")
	hs, err := u.hook.Read()
	if err != nil {
		return nil, err
	} else if hs.Status == hook.Started {
		log.Printf("awaiting error resolution for %q hook", hs.Info.Kind)
		return ModeHookError, nil
	} else if hs.Status == hook.Succeeded {
		log.Printf("recovering uncommitted %q hook", hs.Info.Kind)
		if err = u.commitHook(hs.Info); err != nil {
			return nil, err
		}
	}
	log.Printf("continuing after %q hook", hs.Info.Kind)
	if hs.Info.Kind == hook.Install {
		return ModeStarting, nil
	}
	return ModeStarted, nil
}

// ModeInstalling is responsible for creating the charm directory and running
// the "install" hook.
func ModeInstalling(sch *state.Charm) Mode {
	return func(u *Uniter) (mode Mode, err error) {
		defer trivial.ErrorContextf(&err, "ModeInstalling")
		if err = u.changeCharm(sch, charm.Installing); err != nil {
			return nil, err
		}
		err = u.runHook(hook.Info{Kind: hook.Install})
		if err != nil && err != errHookFailed {
			return nil, err
		}
		return nextMode(u)
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
// * service configuration changes
// * charm upgrade requests (not implemented)
// * relation changes (not implemented)
// * unit death (not implemented)
func ModeStarted(u *Uniter) (mode Mode, err error) {
	defer trivial.ErrorContextf(&err, "ModeStarted")
	if err = u.unit.SetStatus(state.UnitStarted, ""); err != nil {
		return nil, err
	}
	config := u.service.WatchConfig()
	defer stop(config, &mode, &err)
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
			// TODO once we've run the initial config-changed, start other watches.
		}
		// TODO: unit death; charm upgrades; relations.
	}
	panic("unreachable")
}

// ModeHookError is responsible for watching and responding to:
// * user resolution of hook errors
// * forced charm upgrade requests (not implemented)
// * unit death (not implemented)
func ModeHookError(u *Uniter) (mode Mode, err error) {
	defer trivial.ErrorContextf(&err, "ModeHookError")
	hs, err := u.hook.Read()
	if err != nil {
		return nil, err
	}
	if hs.Status != hook.Started {
		return nil, fmt.Errorf("inconsistent hook status %q", hs.Status)
	}
	msg := fmt.Sprintf("hook failed: %q", hs.Info.Kind)
	if err = u.unit.SetStatus(state.UnitError, msg); err != nil {
		return nil, err
	}
	// Wait for shutdown, error resolution, or forced charm upgrade.
	resolved := u.unit.WatchResolved()
	defer stop(resolved, &mode, &err)
	for {
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case rm, ok := <-resolved.Changes():
			if !ok {
				return nil, watcher.MustErr(resolved)
			}
			switch rm {
			case state.ResolvedNone:
				continue
			case state.ResolvedRetryHooks:
				err = u.runHook(hs.Info)
			case state.ResolvedNoHooks:
				err = u.commitHook(hs.Info)
			default:
				panic(fmt.Errorf("unhandled resolved mode %q", rm))
			}
			if e := u.unit.ClearResolved(); e != nil {
				err = e
			}
			if err == errHookFailed {
				continue
			} else if err != nil {
				return nil, err
			}
			return nextMode(u)
		}
		// TODO: unit death; charm upgrades.
	}
	panic("unreachable")
}

// stop is used by Mode funcs to shut down watchers on return.
func stop(s stopper, mode *Mode, err *error) {
	if e := s.Stop(); e != nil && err == nil {
		*mode = nil
		*err = e
	}
}

type stopper interface {
	Stop() error
}
