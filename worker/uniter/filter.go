package uniter

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
)

// filter collects unit, service, and service config information from separate
// state watchers, and presents it as events on channels designed specifically
// for the convenience of the uniter.
type filter struct {
	st           *state.State
	tomb         tomb.Tomb
	outUnitDying chan struct{}
	outResolved  chan state.ResolvedMode
	outConfig    chan struct{}
	outUpgrade   chan *state.Charm
	wantResolved chan struct{}
	wantUpgrade  chan charmChange
	wantConfig   chan struct{}
}

// charmChange holds the result of a service's CharmURL method.
type charmChange struct {
	url   *charm.URL
	force bool
}

// newFilter returns a filter that handles state changes pertaining to the
// supplied unit.
func newFilter(st *state.State, unitName string) (*filter, error) {
	f := &filter{
		st:           st,
		outUnitDying: make(chan struct{}),
		outResolved:  make(chan state.ResolvedMode),
		outUpgrade:   make(chan *state.Charm),
		outConfig:    make(chan struct{}),
		wantResolved: make(chan struct{}),
		wantConfig:   make(chan struct{}),
		wantUpgrade:  make(chan charmChange),
	}
	go func() {
		defer f.tomb.Done()
		err := f.loop(unitName)
		log.Printf("filter error: %v", err)
		f.tomb.Kill(err)
	}()
	return f, nil
}

func (f *filter) loop(unitName string) (err error) {
	unit, err := f.st.Unit(unitName)
	if err != nil {
		return err
	}
	life := state.Alive
	rm := state.ResolvedNone
	var outResolved chan<- state.ResolvedMode
	unitChanged := func() error {
		if life != unit.Life() {
			switch life = unit.Life(); life {
			case state.Dying:
				log.Printf("unit is dying")
				close(f.outUnitDying)
			case state.Dead:
				log.Printf("unit is dead")
				return worker.ErrDead
			}
		}
		if rm_ := unit.Resolved(); rm_ != rm {
			rm = rm_
			if rm != state.ResolvedNone {
				outResolved = f.outResolved
			}
		}
		return nil
	}
	if err = unitChanged(); err != nil {
		return err
	}

	service, err := unit.Service()
	if err != nil {
		return err
	}
	url, force := service.CharmURL()
	gotCharm := charmChange{url, force}
	upgradeFrom := gotCharm
	var upgrade *state.Charm
	var outUpgrade chan<- *state.Charm
	updateUpgrade := func() (err error) {
		if life != state.Alive {
			log.Debugf("filter: charm check skipped, unit is dying")
			outUpgrade = nil
			return nil
		}
		if *upgradeFrom.url != *gotCharm.url {
			if gotCharm.force || !upgradeFrom.force {
				log.Debugf("filter: preparing new charm event")
				upgrade, err = f.st.Charm(gotCharm.url)
				if err != nil {
					return err
				}
				outUpgrade = f.outUpgrade
			}
		}
		log.Debugf("filter: no new charm event")
		return nil
	}
	serviceChanged := func() error {
		url, force := service.CharmURL()
		gotCharm = charmChange{url, force}
		switch service.Life() {
		case state.Dying:
			if err = unit.EnsureDying(); err != nil {
				return err
			}
		case state.Dead:
			return fmt.Errorf("service unexpectedly dead")
		}
		return updateUpgrade()
	}
	if err := serviceChanged(); err != nil {
		return err
	}

	// Set up watchers for unit, service, and config.
	unitw := unit.Watch()
	defer watcher.Stop(unitw, &f.tomb)
	servicew := service.Watch()
	defer watcher.Stop(servicew, &f.tomb)
	configw := service.WatchConfig()
	defer watcher.Stop(configw, &f.tomb)

	// Consume the initial config change so that we can send a config
	// event immediately, and only send future changes from this state,
	// without introducing complexity below.
	var outConfig chan struct{}
	select {
	case <-f.tomb.Dying():
		return tomb.ErrDying
	case <-configw.Changes():
		outConfig = f.outConfig
	}

	for {
		var ok bool
		select {
		case <-f.tomb.Dying():
			return tomb.ErrDying

		// Handle watcher changes.
		case unit, ok = <-unitw.Changes():
			log.Debugf("filter: got unit change")
			if !ok {
				return watcher.MustErr(unitw)
			}
			if err = unitChanged(); err != nil {
				return err
			}
		case service, ok = <-servicew.Changes():
			log.Debugf("filter: got service change")
			if !ok {
				return watcher.MustErr(servicew)
			}
			if err = serviceChanged(); err != nil {
				return err
			}
		case _, ok := <-configw.Changes():
			log.Debugf("filter got config change")
			if !ok {
				return watcher.MustErr(configw)
			}
			log.Debugf("filter: preparing new config event")
			outConfig = f.outConfig

		// Send events on active out chans.
		case outResolved <- rm:
			log.Debugf("filter: sent resolved event")
			outResolved = nil
		case outConfig <- nothing:
			log.Debugf("filter: sent config event")
			outConfig = nil
		case outUpgrade <- upgrade:
			log.Debugf("filter: sent charm event")
			upgradeFrom.url = upgrade.URL()
			outUpgrade = nil

		// On request, ensure that if an event is available it will be resent.
		case <-f.wantResolved:
			log.Debugf("filter: want resolved event")
			if rm != state.ResolvedNone {
				outResolved = f.outResolved
			}
		case <-f.wantConfig:
			log.Debugf("filter: want config event")
			outConfig = f.outConfig
		case upgradeFrom = <-f.wantUpgrade:
			log.Debugf("filter: want charm event")
			if err = updateUpgrade(); err != nil {
				return err
			}
		}
	}
	panic("unreachable")
}

func (f *filter) Stop() error {
	f.tomb.Kill(nil)
	return f.tomb.Wait()
}

func (f *filter) Dying() <-chan struct{} {
	return f.tomb.Dying()
}

func (f *filter) Wait() error {
	return f.tomb.Wait()
}

// UnitDying returns a channel which is closed when the Unit enters a Dying state.
func (f *filter) UnitDying() <-chan struct{} {
	return f.outUnitDying
}

// ResolvedEvents returns a channel that may receive a ResolvedMode when the
// unit's Resolved value changes, or when an event is explicitly requested.
// A ResolvedNone state will never generate events, but ResolvedRetryHooks and
// ResolvedNoHooks will always be delivered as described.
func (f *filter) ResolvedEvents() <-chan state.ResolvedMode {
	return f.outResolved
}

// ConfigEvents returns a channel that will receive a signal whenever the service's
// configuration changes, or when an event is explicitly requested.
func (f *filter) ConfigEvents() <-chan struct{} {
	return f.outConfig
}

// UpgradeEvents returns a channel that will receive a new charm whenever an
// upgrade is indicated. Events should not be read until the baseline state
// has been specified by calling WantUpgradeEvent.
func (f *filter) UpgradeEvents() <-chan *state.Charm {
	return f.outUpgrade
}

// WantResolvedEvent indicates that the filter should send a resolved event
// if one is available.
func (f *filter) WantResolvedEvent() {
	select {
	case <-f.tomb.Dying():
	case f.wantResolved <- nothing:
	}
}

// WantConfigEvent indicates that the filter should send a config event.
func (f *filter) WantConfigEvent() {
	select {
	case <-f.tomb.Dying():
	case f.wantConfig <- nothing:
	}
}

// WantUpgradeEvent sets the baseline from which service charm changes will
// be considered for upgrade. Any service charm with a URL different from
// that supplied will be considered; if mustForce is true, unforced service
// charms will be ignored.
func (f *filter) WantUpgradeEvent(url *charm.URL, mustForce bool) {
	select {
	case <-f.tomb.Dying():
	case f.wantUpgrade <- charmChange{url, mustForce}:
	}
}

// nothing is marginally more pleasant to read than "struct{}{}".
var nothing = struct{}{}
