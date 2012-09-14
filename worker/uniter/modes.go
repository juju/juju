package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/environs"
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
	log.Printf("updating unit addresses")
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	provider, err := environs.Provider(cfg.Type())
	if err != nil {
		return nil, err
	}
	if private, err := provider.PrivateAddress(); err != nil {
		return nil, err
	} else if err = u.unit.SetPrivateAddress(private); err != nil {
		return nil, err
	}
	if public, err := provider.PublicAddress(); err != nil {
		return nil, err
	} else if err = u.unit.SetPublicAddress(public); err != nil {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeContinue determines what action to take based on persistent uniter state.
func ModeContinue(u *Uniter) (next Mode, err error) {
	defer errorContextf(&err, "ModeContinue")

	// When no charm exists, install it.
	log.Printf("reading uniter state from disk...")
	s, err := u.sf.Read()
	if err == ErrNoStateFile {
		log.Printf("charm is not deployed")
		sch, _, err := u.service.Charm()
		if err != nil {
			return nil, err
		}
		return ModeInstalling(sch), nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read charm state: %v", err)
	}

	// Filter out states not related to charm deployment.
	switch s.Op {
	case Abide:
		log.Printf("continuing after %q hook", s.Hook.Kind)
		if s.Hook.Kind == hook.Install {
			return ModeStarting, nil
		}
		return ModeStarted, nil
	case RunHook:
		if s.OpStep == Queued {
			log.Printf("running queued %q hook", s.Hook.Kind)
			if err := u.runHook(*s.Hook); err != nil {
				if err != errHookFailed {
					return nil, err
				}
			}
			return ModeContinue, nil
		}
		if s.OpStep == Done {
			log.Printf("recovering uncommitted %q hook", s.Hook.Kind)
			if err = u.commitHook(*s.Hook); err != nil {
				return nil, err
			}
			return ModeContinue, nil
		}
		log.Printf("awaiting error resolution for %q hook", s.Hook.Kind)
		return ModeHookError, nil
	}

	// Resume interrupted deployment operations.
	sch, err := u.st.Charm(s.CharmURL)
	if err != nil {
		return nil, err
	}
	if s.Op == Install {
		log.Printf("resuming charm install")
		return ModeInstalling(sch), nil
	} else if s.Op == Upgrade {
		log.Printf("resuming charm upgrade")
		return ModeUpgrading(sch), nil
	}
	panic(fmt.Errorf("unhandled uniter operation %q", s.Op))
}

// ModeInstalling is responsible for the initial charm deployment.
func ModeInstalling(sch *state.Charm) Mode {
	return func(u *Uniter) (next Mode, err error) {
		defer errorContextf(&err, "ModeInstalling")
		if err = u.deploy(sch, Install); err != nil {
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
	s, err := u.sf.Read()
	if err != nil {
		return nil, err
	}
	if s.Op != Abide {
		return nil, fmt.Errorf("insane uniter state: %#v", s)
	}
	if err = u.unit.SetStatus(state.UnitStarted, ""); err != nil {
		return nil, err
	}

	// To begin with, only watch for config changes, and exploit the
	// guaranteed initial send to ensure we run a config-changed hook
	// before starting any other watches.
	starting := true
	configw := u.service.WatchConfig()
	defer stop(configw, &next, &err)
	var charmw *state.ServiceCharmWatcher
	var charms <-chan state.ServiceCharmChange
	for {
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case _, ok := <-configw.Changes():
			if !ok {
				return nil, watcher.MustErr(configw)
			}
			hi := hook.Info{Kind: hook.ConfigChanged}
			if err = u.runHook(hi); err != nil {
				if err == errHookFailed {
					return ModeHookError, nil
				}
				return nil, err
			}
			if starting {
				// If we haven't already set up additional watches, do so now.
				starting = false
				charmw = u.service.WatchCharm()
				defer stop(charmw, &next, &err)
				charms = charmw.Changes()
			}
		case ch, ok := <-charms:
			if !ok {
				return nil, watcher.MustErr(charmw)
			}
			url, err := charm.ReadCharmURL(u.charm)
			if err != nil {
				return nil, err
			}
			if *ch.Charm.URL() != *url {
				return ModeUpgrading(ch.Charm), nil
			}
		}
		// TODO: unit death; relations.
	}
	panic("unreachable")
}

// ModeHookError is responsible for watching and responding to:
// * user resolution of hook errors
// * forced charm upgrade requests (not implemented)
// * unit death (not implemented)
func ModeHookError(u *Uniter) (next Mode, err error) {
	defer errorContextf(&err, "ModeHookError")
	s, err := u.sf.Read()
	if err != nil {
		return nil, err
	}
	if s.Op != RunHook || s.OpStep != Pending {
		return nil, fmt.Errorf("insane uniter state: %#v", s)
	}
	msg := fmt.Sprintf("hook failed: %q", s.Hook.Kind)
	if err = u.unit.SetStatus(state.UnitError, msg); err != nil {
		return nil, err
	}

	// Wait for shutdown, error resolution, or forced charm upgrade.
	resolvedw := u.unit.WatchResolved()
	defer stop(resolvedw, &next, &err)
	charmw := u.service.WatchCharm()
	defer stop(charmw, &next, &err)
	for {
		select {
		case <-u.tomb.Dying():
			return nil, tomb.ErrDying
		case rm, ok := <-resolvedw.Changes():
			if !ok {
				return nil, watcher.MustErr(resolvedw)
			}
			switch rm {
			case state.ResolvedNone:
				continue
			case state.ResolvedRetryHooks:
				err = u.runHook(*s.Hook)
			case state.ResolvedNoHooks:
				err = u.commitHook(*s.Hook)
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
		case ch, ok := <-charmw.Changes():
			if !ok {
				return nil, watcher.MustErr(charmw)
			}
			url, err := charm.ReadCharmURL(u.charm)
			if err != nil {
				return nil, err
			}
			if ch.Force && *ch.Charm.URL() != *url {
				return ModeUpgrading(ch.Charm), nil
			}
		}
		// TODO: unit death.
	}
	panic("unreachable")
}

// ModeUpgrading is responsible for upgrading the charm.
func ModeUpgrading(sch *state.Charm) Mode {
	return func(u *Uniter) (Mode, error) {
		log.Printf("upgrading charm to %q", sch.URL())
		if err := u.deploy(sch, Upgrade); err != nil {
			if err == charm.ErrConflict {
				return ModeConflicted(sch), nil
			}
			return nil, err
		}
		return ModeContinue, nil
	}
}

// ModeConflicted waits for the user to resolve an error encountered when
// upgrading a charm. This may be done either by manually resolving errors
// and then setting the resolved flag, or by forcing an upgrade to a
// different charm.
func ModeConflicted(sch *state.Charm) Mode {
	return func(u *Uniter) (next Mode, err error) {
		if err = u.unit.SetStatus(state.UnitError, "upgrade failed"); err != nil {
			return nil, err
		}
		resolvedw := u.unit.WatchResolved()
		defer stop(resolvedw, &next, &err)
		charmw := u.service.WatchCharm()
		defer stop(charmw, &next, &err)
		for {
			select {
			case <-u.tomb.Dying():
				return nil, tomb.ErrDying
			case ch, ok := <-charmw.Changes():
				if !ok {
					return nil, watcher.MustErr(charmw)
				}
				if ch.Force && *ch.Charm.URL() != *sch.URL() {
					if err := u.charm.Revert(); err != nil {
						return nil, err
					}
					return ModeUpgrading(ch.Charm), nil
				}
			case rm, ok := <-resolvedw.Changes():
				if !ok {
					return nil, watcher.MustErr(resolvedw)
				}
				if rm == state.ResolvedNone {
					continue
				}
				err := u.charm.Snapshotf("Upgrade conflict resolved.")
				if e := u.unit.ClearResolved(); e != nil && err == nil {
					err = e
				}
				if err != nil {
					return nil, err
				}
				return ModeUpgrading(sch), nil
			}
			// TODO: unit death.
		}
		panic("unreachable")
	}
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
// or if err is tomb.ErrDying, errorContextf does nothing.
func errorContextf(err *error, format string, args ...interface{}) {
	if *err != nil && *err != tomb.ErrDying {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}
