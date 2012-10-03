package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/uniter/charm"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/tomb"
)

// Mode defines the signature of the functions that implement the possible
// states of a running Uniter.
type Mode func(u *Uniter) (Mode, error)

// ModeInit is the initial Uniter mode.
func ModeInit(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeInit", &next, &err)()
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
	defer modeContext("ModeContinue", &next, &err)()

	// When no charm exists, install it.
	s, err := u.sf.Read()
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
	switch s.Op {
	case Abide:
		log.Printf("continuing after %q hook", s.Hook.Kind)
		switch s.Hook.Kind {
		case hook.Install:
			return ModeStarting, nil
		case hook.Stop:
			return ModeTerminating, nil
		}
		return ModeAbide, nil
	case RunHook:
		if s.OpStep == Queued {
			log.Printf("found queued %q hook", s.Hook.Kind)
			return nil, u.runHook(*s.Hook)
		}
		if s.OpStep == Done {
			log.Printf("found uncommitted %q hook", s.Hook.Kind)
			return nil, u.commitHook(*s.Hook)
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
		name := fmt.Sprintf("ModeInstalling %s", sch.URL())
		defer modeContext(name, &next, &err)()
		return nil, u.deploy(sch, Install)
	}
}

// ModeUpgrading is responsible for upgrading the charm.
func ModeUpgrading(sch *state.Charm) Mode {
	return func(u *Uniter) (next Mode, err error) {
		name := fmt.Sprintf("ModeUpgrading %s", sch.URL())
		defer modeContext(name, &next, &err)()
		if err = u.deploy(sch, Upgrade); err == charm.ErrConflict {
			return ModeConflicted(sch), nil
		}
		return nil, err
	}
}

// ModeStarting runs the "start" hook.
func ModeStarting(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeStarting", &next, &err)()
	if err = u.unit.SetStatus(state.UnitInstalled, ""); err != nil {
		return nil, err
	}
	return nil, u.runHook(hook.Info{Kind: hook.Start})
}

// ModeStopping runs the "stop" hook.
func ModeStopping(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeStopping", &next, &err)()
	return nil, u.runHook(hook.Info{Kind: hook.Stop})
}

// ModeTerminating marks the unit dead and returns ErrDead.
func ModeTerminating(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeTerminating", &next, &err)()
	if err = u.unit.SetStatus(state.UnitStopped, ""); err != nil {
		return nil, err
	}
	if err = u.unit.EnsureDead(); err != nil {
		return nil, err
	}
	return nil, worker.ErrDead
}

// ModeAbide is the Uniter's usual steady state. It watches for and responds to:
// * service configuration changes
// * charm upgrade requests
// * relation changes (not implemented)
// * unit death
func ModeAbide(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeAbide", &next, &err)()
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

	// Execute an initial config-changed hook regardless of state.
	cc := hook.Info{Kind: hook.ConfigChanged}
	u.wantConfigEvent()
	select {
	case <-u.Dying():
		return nil, tomb.ErrDying
	case <-u.configEvents():
		if err = u.runHook(cc); err != nil {
			return nil, err
		}
	}

	// Watch for everything else (including further config changes).
	u.wantCharmEvent()
	for {
		select {
		case <-u.Dying():
			return nil, tomb.ErrDying
		case <-u.unitDying():
			// TODO don't stop until all relations broken.
			return ModeStopping, nil
		case <-u.configEvents():
			if err = u.runHook(cc); err != nil {
				return nil, err
			}
		case ch := <-u.charmEvents():
			upgrade, err := u.getUpgrade(ch, false)
			if err == errNoUpgrade {
				continue
			} else if err != nil {
				return nil, err
			}
			return ModeUpgrading(upgrade), nil
		}
	}
	panic("unreachable")
}

// ModeHookError is responsible for watching and responding to:
// * user resolution of hook errors
// * charm upgrade requests
func ModeHookError(u *Uniter) (next Mode, err error) {
	defer modeContext("ModeHookError", &next, &err)()
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
	resolveHook := getResolveHook(*s.Hook)
	u.wantResolvedEvent()
	u.wantCharmEvent()
	for {
		select {
		case <-u.Dying():
			return nil, tomb.ErrDying
		case rm := <-u.resolvedEvents():
			if success, err := u.resolveError(*rm, resolveHook); success {
				return ModeContinue, nil
			} else if err != nil && err != errHookFailed {
				return nil, err
			}
		case ch := <-u.charmEvents():
			upgrade, err := u.getUpgrade(ch, true)
			if err == errNoUpgrade {
				continue
			} else if err != nil {
				return nil, err
			}
			return ModeUpgrading(upgrade), nil
		}
	}
	panic("unreachable")
}

// ModeConflicted is responsible for watching and responding to:
// * user resolution of charm upgrade conflicts
// * forced charm upgrade requests
func ModeConflicted(sch *state.Charm) Mode {
	return func(u *Uniter) (next Mode, err error) {
		defer modeContext("ModeConflicted", &next, &err)()
		if err = u.unit.SetStatus(state.UnitError, "upgrade failed"); err != nil {
			return nil, err
		}
		u.wantResolvedEvent()
		u.wantCharmEvent()
		for {
			select {
			case <-u.Dying():
				return nil, tomb.ErrDying
			case rm := <-u.resolvedEvents():
				if success, err := u.resolveError(*rm, resolveConflict); success {
					return ModeUpgrading(sch), nil
				} else if err != nil {
					return nil, err
				}
			case ch := <-u.charmEvents():
				upgrade, err := u.getUpgrade(ch, true)
				if err != nil {
					if err == errNoUpgrade {
						continue
					}
					return nil, err
				}
				if err := u.charm.Revert(); err != nil {
					return nil, err
				}
				return ModeUpgrading(upgrade), nil
			}
		}
		panic("unreachable")
	}
}

// modeContext returns a function that implements logging and common error
// manipulation for Mode funcs.
func modeContext(name string, next *Mode, err *error) func() {
	log.Printf(name + " starting")
	return func() {
		log.Debugf(name + " exiting")
		switch *err {
		case nil:
			if *next == nil {
				*next = ModeContinue
			}
		case errHookFailed:
			*next, *err = ModeHookError, nil
		case tomb.ErrDying, worker.ErrDead:
			log.Printf(name + " shutting down")
		default:
			*err = errors.New(name + ": " + (*err).Error())
		}
	}
}
