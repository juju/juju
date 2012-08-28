package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker/uniter/charm"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/tomb"
)

// Mode defines the signature of the functions that implement the possible
// states of a running Uniter.
type Mode func(u *Uniter) (Mode, error)

// ModeInit is the initial Uniter mode.
func ModeInit(u *Uniter) (next Mode, err error) {
	defer errorContextf(&err, "ModeInit")
	log.Printf("examining charm state...")
	var sch *state.Charm
	cs, err := u.charm.ReadState()
	if err == charm.ErrMissing {
		log.Printf("charm is not deployed")
		if sch, _, err = u.service.Charm(); err != nil {
			return nil, err
		}
		return ModeInstalling(sch), nil
	} else if err != nil {
		return nil, err
	}
	if cs.Status == charm.Deployed {
		log.Printf("charm is deployed")
		return ModeContinue, nil
	} else if sch, err = u.st.Charm(cs.URL); err != nil {
		return nil, err
	}
	switch cs.Status {
	case charm.Installing:
		log.Printf("resuming charm install")
		return ModeInstalling(sch), nil
	case charm.Upgrading, charm.Conflicted:
		panic("not implemented")
	}
	panic("unreachable")
}

// ModeContinue determines what action to take based on hook status.
func ModeContinue(u *Uniter) (next Mode, err error) {
	defer errorContextf(&err, "ModeContinue")
	log.Printf("examining hook state...")
	hs, err := u.hook.Read()
	if err != nil {
		return nil, err
	}
	switch hs.Status {
	case hook.Pending:
		log.Printf("awaiting error resolution for %q hook", hs.Info.Kind)
		return ModeHookError, nil
	case hook.Committing:
		log.Printf("recovering uncommitted %q hook", hs.Info.Kind)
		if err = u.commitHook(hs.Info); err != nil {
			return nil, err
		}
		return ModeContinue, nil
	case hook.Queued:
		log.Printf("running queued %q hook", hs.Info.Kind)
		if err := u.runHook(hs.Info); err != nil {
			if err == errHookFailed {
				return ModeHookError, nil
			}
			return nil, err
		}
		return ModeContinue, nil
	case hook.Complete:
		log.Printf("continuing after %q hook", hs.Info.Kind)
		if hs.Info.Kind == hook.Install {
			return ModeStarting, nil
		}
		return ModeStarted, nil
	}
	panic(fmt.Errorf("unhandled hook status %q", hs.Status))
}

// ModeInstalling is responsible for creating the charm directory and running
// the "install" hook.
func ModeInstalling(sch *state.Charm) Mode {
	return func(u *Uniter) (next Mode, err error) {
		defer errorContextf(&err, "ModeInstalling")
		if err = u.changeCharm(sch, charm.Installing); err != nil {
			return nil, err
		}
		return ModeContinue, nil
	}
}

// ModeStarting is responsible for running the "start" hook.
func ModeStarting(u *Uniter) (next Mode, err error) {
	defer errorContextf(&err, "ModeStarting")
	if err := u.unit.SetStatus(state.UnitInstalled, ""); err != nil {
		return nil, err
	}
	hi := hook.Info{Kind: hook.Start}
	if err := u.runHook(hi); err != nil && err != errHookFailed {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeStarted is the Uniter's usual steady state. It watches for and responds to:
// * service configuration changes
// * charm upgrade requests (not implemented)
// * relation changes (not implemented)
// * unit death (not implemented)
func ModeStarted(u *Uniter) (next Mode, err error) {
	defer errorContextf(&err, "ModeStarted")
	if err = u.unit.SetStatus(state.UnitStarted, ""); err != nil {
		return nil, err
	}
	config := u.service.WatchConfig()
	defer stop(config, &next, &err)
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
func ModeHookError(u *Uniter) (next Mode, err error) {
	defer errorContextf(&err, "ModeHookError")
	hs, err := u.hook.Read()
	if err != nil {
		return nil, err
	}
	if hs.Status != hook.Pending {
		return nil, fmt.Errorf("inconsistent hook status %q", hs.Status)
	}
	msg := fmt.Sprintf("hook failed: %q", hs.Info.Kind)
	if err = u.unit.SetStatus(state.UnitError, msg); err != nil {
		return nil, err
	}
	// Wait for shutdown, error resolution, or forced charm upgrade.
	resolved := u.unit.WatchResolved()
	defer stop(resolved, &next, &err)
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
			return ModeContinue, nil
		}
		// TODO: unit death; charm upgrades.
	}
	panic("unreachable")
}

// stop is used by Mode funcs to shut down watchers on return.
func stop(s stopper, next *Mode, err *error) {
	if e := s.Stop(); e != nil && *err == nil {
		*next = nil
		*err = e
	}
}

type stopper interface {
	Stop() error
}

// errorContextf prefixes the error stored in err with text formatted
// according to the format specifier. If err does not contain an error,
// or if err is tome.ErrDying, errorContextf does nothing.
func errorContextf(err *error, format string, args ...interface{}) {
	if *err != nil && *err != tomb.ErrDying {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}
