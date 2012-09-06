package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
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

	if err := u.charm.Recover(); err != nil {
		return nil, err
	}
	return ModeContinue, nil
}

// ModeContinue determines what action to take based on persistent uniter state.
func ModeContinue(u *Uniter) (next Mode, err error) {
	defer errorContextf(&err, "ModeContinue")

	// When no charm exists, install it.
	log.Printf("examining persistent state...")
	op, err := u.op.Read()
	if err == ErrNoStateFile {
		log.Printf("charm is not deployed")
		sch, _, err := u.service.Charm()
		if err != nil {
			return nil, err
		}
		return ModeInstalling(sch), nil
	} else if err != nil {
		return nil, err
	}

	// Filter out states not related to charm deployment.
	switch op.Op {
	case Abide:
		log.Printf("continuing after %q hook", op.Hook.Kind)
		if op.Hook.Kind == hook.Install {
			return ModeStarting, nil
		}
		return ModeStarted, nil
	case RunHook:
		if op.Status == Queued {
			log.Printf("running queued %q hook", op.Hook.Kind)
			if err := u.runHook(*op.Hook); err != nil {
				if err != errHookFailed {
					return nil, err
				}
			}
			return ModeContinue, nil
		}
		if op.Status == Committing {
			log.Printf("recovering uncommitted %q hook", op.Hook.Kind)
			if err = u.commitHook(*op.Hook); err != nil {
				return nil, err
			}
			return ModeContinue, nil
		}
		log.Printf("awaiting error resolution for %q hook", op.Hook.Kind)
		return ModeHookError, nil
	}

	// Resume interrupted deployment operations.
	sch, err := u.st.Charm(op.CharmURL)
	if err != nil {
		return nil, err
	}
	if op.Op == Install {
		log.Printf("resuming charm install")
		return ModeInstalling(sch), nil
	}
	panic(fmt.Errorf("unhandled operation %q", op.Op))
}

// ModeInstalling is responsible for creating the charm directory and running
// the "install" hook.
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
	op, err := u.op.Read()
	if err != nil {
		return nil, err
	}
	if op.Op != Abide {
		return nil, fmt.Errorf("insane uniter state: %#v", op)
	}
	if err = u.unit.SetStatus(state.UnitStarted, ""); err != nil {
		return nil, err
	}
	configw := u.service.WatchConfig()
	defer stop(configw, &next, &err)
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
	op, err := u.op.Read()
	if err != nil {
		return nil, err
	}
	if op.Op != RunHook || op.Status != Pending {
		return nil, fmt.Errorf("insane uniter state: %#v", op)
	}
	msg := fmt.Sprintf("hook failed: %q", op.Hook.Kind)
	if err = u.unit.SetStatus(state.UnitError, msg); err != nil {
		return nil, err
	}
	// Wait for shutdown, error resolution, or forced charm upgrade.
	resolvedw := u.unit.WatchResolved()
	defer stop(resolvedw, &next, &err)
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
				err = u.runHook(*op.Hook)
			case state.ResolvedNoHooks:
				err = u.commitHook(*op.Hook)
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
// or if err is tomb.ErrDying, errorContextf does nothing.
func errorContextf(err *error, format string, args ...interface{}) {
	if *err != nil && *err != tomb.ErrDying {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}
