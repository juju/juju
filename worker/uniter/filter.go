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
	outCharm     chan *state.Charm
	wantResolved chan struct{}
	wantConfig   chan struct{}
	wantCharm    chan charmChange
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
		outConfig:    make(chan struct{}),
		outCharm:     make(chan *state.Charm),
		wantResolved: make(chan struct{}),
		wantConfig:   make(chan struct{}),
		wantCharm:    make(chan charmChange),
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
	var outCharm chan<- *state.Charm
	updateCharm := func() (err error) {
		if life != state.Alive {
			outCharm = nil
			return nil
		}
		if *upgradeFrom.url != *gotCharm.url {
			if gotCharm.force || !upgradeFrom.force {
				log.Debugf("filter: preparing new charm event")
				upgrade, err = f.st.Charm(gotCharm.url)
				if err != nil {
					return err
				}
				outCharm = f.outCharm
			}
		}
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
		return updateCharm()
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
		case outCharm <- upgrade:
			log.Debugf("filter: sent charm event")
			upgradeFrom.url = upgrade.URL()
			outCharm = nil

		// On request, ensure that if an event is available it will be resent.
		case <-f.wantResolved:
			log.Debugf("filter: want resolved event")
			if rm != state.ResolvedNone {
				outResolved = f.outResolved
			}
		case <-f.wantConfig:
			log.Debugf("filter: want config event")
			outConfig = f.outConfig
		case upgradeFrom = <-f.wantCharm:
			log.Debugf("filter: want charm event")
			if err = updateCharm(); err != nil {
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

// unitDying returns a channel which is closed when the Unit enters a Dying state.
func (f *filter) unitDying() <-chan struct{} {
	return f.outUnitDying
}

// resolvedEvents returns a channel that will receive a ResolvedMode whenever the
// unit's Resolved value changes, or when an event is explicitly requested.
func (f *filter) resolvedEvents() <-chan state.ResolvedMode {
	return f.outResolved
}

// configEvents returns a channel that will receive a signal whenever the service's
// configuration changes, or when an event is explicitly requested.
func (f *filter) configEvents() <-chan struct{} {
	return f.outConfig
}

// charmEvents returns a channel that will receive a charmChange whenever the
// service's charm changes, or when an event is explicitly requested.
func (f *filter) charmEvents() <-chan *state.Charm {
	return f.outCharm
}

// wantResolvedEvent indicates that the filter should send a resolved event
// if one is available.
func (f *filter) wantResolvedEvent() {
	select {
	case <-f.tomb.Dying():
	case f.wantResolved <- nothing:
	}
}

// wantConfigEvent indicates that the filter should send a config event.
func (f *filter) wantConfigEvent() {
	select {
	case <-f.tomb.Dying():
	case f.wantConfig <- nothing:
	}
}

// wantCharmEvent indicates that the filter should send a charm event, if the
// supplied state is such that the current service charm is a valid upgrade.
func (f *filter) wantCharmEvent(url *charm.URL, mustForce bool) {
	select {
	case <-f.tomb.Dying():
	case f.wantCharm <- charmChange{url, mustForce}:
	}
}

// nothing is marginally more pleasant to read than "struct{}{}".
var nothing = struct{}{}
