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
	st   *state.State
	tomb tomb.Tomb

	// outUnitDying is closed when the unit's life becomes Dying.
	outUnitDying chan struct{}

	// The out*On chans are used to deliver events to clients.
	// The out* chans, when set to the corresponding out*On chan (rather than
	// nil) indicate that an event of the appropriate type is ready to send
	// to the client.
	outConfig     chan struct{}
	outConfigOn   chan struct{}
	outUpgrade    chan *state.Charm
	outUpgradeOn  chan *state.Charm
	outResolved   chan state.ResolvedMode
	outResolvedOn chan state.ResolvedMode

	// The want* chans are used to indicate that the filter should send
	// events if it has them available.
	wantConfig   chan struct{}
	wantUpgrade  chan serviceCharm
	wantResolved chan struct{}

	// The following fields hold state that is collected while running,
	// and used to detect interesting changes to express as events.
	unit             *state.Unit
	life             state.Life
	resolved         state.ResolvedMode
	service          *state.Service
	upgradeRequested serviceCharm
	upgradeAvailable serviceCharm
	upgrade          *state.Charm
}

// newFilter returns a filter that handles state changes pertaining to the
// supplied unit.
func newFilter(st *state.State, unitName string) (*filter, error) {
	f := &filter{
		st:            st,
		outUnitDying:  make(chan struct{}),
		outConfig:     make(chan struct{}),
		outConfigOn:   make(chan struct{}),
		outUpgrade:    make(chan *state.Charm),
		outUpgradeOn:  make(chan *state.Charm),
		outResolved:   make(chan state.ResolvedMode),
		outResolvedOn: make(chan state.ResolvedMode),
		wantResolved:  make(chan struct{}),
		wantConfig:    make(chan struct{}),
		wantUpgrade:   make(chan serviceCharm),
	}
	go func() {
		defer f.tomb.Done()
		err := f.loop(unitName)
		log.Printf("worker/uniter: filter error: %v", err)
		f.tomb.Kill(err)
	}()
	return f, nil
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

// ConfigEvents returns a channel that will receive a signal whenever the service's
// configuration changes, or when an event is explicitly requested.
func (f *filter) ConfigEvents() <-chan struct{} {
	return f.outConfigOn
}

// UpgradeEvents returns a channel that will receive a new charm whenever an
// upgrade is indicated. Events should not be read until the baseline state
// has been specified by calling WantUpgradeEvent.
func (f *filter) UpgradeEvents() <-chan *state.Charm {
	return f.outUpgradeOn
}

// ResolvedEvents returns a channel that may receive a ResolvedMode when the
// unit's Resolved value changes, or when an event is explicitly requested.
// A ResolvedNone state will never generate events, but ResolvedRetryHooks and
// ResolvedNoHooks will always be delivered as described.
func (f *filter) ResolvedEvents() <-chan state.ResolvedMode {
	return f.outResolvedOn
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
	case f.wantUpgrade <- serviceCharm{url, mustForce}:
	}
}

// WantResolvedEvent indicates that the filter should send a resolved event
// if one is available.
func (f *filter) WantResolvedEvent() {
	select {
	case <-f.tomb.Dying():
	case f.wantResolved <- nothing:
	}
}

func (f *filter) loop(unitName string) (err error) {
	f.unit, err = f.st.Unit(unitName)
	if err != nil {
		return err
	}
	if err = f.unitChanged(); err != nil {
		return err
	}
	f.service, err = f.unit.Service()
	if err != nil {
		return err
	}
	f.upgradeRequested.url, _ = f.service.CharmURL()
	if err = f.serviceChanged(); err != nil {
		return err
	}
	unitw := f.unit.Watch()
	defer watcher.Stop(unitw, &f.tomb)
	servicew := f.service.Watch()
	defer watcher.Stop(servicew, &f.tomb)
	configw := f.service.WatchConfig()
	defer watcher.Stop(configw, &f.tomb)

	// Consume first event so that this sequence doesn't happen:
	//
	// 1) WantConfigEvent called; channel unblocked
	// 2) Config event sent
	// 3) Initial config event received; channel unblocked
	// 4) Config event sent again, for the same configuration
	select {
	case <-f.tomb.Dying():
		return tomb.ErrDying
	case <-configw.Changes():
		f.outConfig = f.outConfigOn
	}

	for {
		var ok bool
		select {
		case <-f.tomb.Dying():
			return tomb.ErrDying

		// Handle watcher changes.
		case f.unit, ok = <-unitw.Changes():
			log.Debugf("worker/uniter/filter: got unit change")
			if !ok {
				return watcher.MustErr(unitw)
			}
			if err = f.unitChanged(); err != nil {
				return err
			}
		case f.service, ok = <-servicew.Changes():
			log.Debugf("worker/uniter/filter: got service change")
			if !ok {
				return watcher.MustErr(servicew)
			}
			if err = f.serviceChanged(); err != nil {
				return err
			}
		case _, ok := <-configw.Changes():
			log.Debugf("worker/uniter/filter got config change")
			if !ok {
				return watcher.MustErr(configw)
			}
			log.Debugf("worker/uniter/filter: preparing new config event")
			f.outConfig = f.outConfigOn

		// Send events on active out chans.
		case f.outConfig <- nothing:
			log.Debugf("worker/uniter/filter: sent config event")
			f.outConfig = nil
		case f.outUpgrade <- f.upgrade:
			log.Debugf("worker/uniter/filter: sent upgrade event")
			f.upgradeRequested.url = f.upgrade.URL()
			f.outUpgrade = nil
		case f.outResolved <- f.resolved:
			log.Debugf("worker/uniter/filter: sent resolved event")
			f.outResolved = nil

		// Handle explicit requests for events.
		case <-f.wantConfig:
			log.Debugf("worker/uniter/filter: want config event")
			f.outConfig = f.outConfigOn
		case req := <-f.wantUpgrade:
			log.Debugf("worker/uniter/filter: want upgrade event")
			f.upgradeRequested = req
			if err = f.upgradeChanged(); err != nil {
				return err
			}
		case <-f.wantResolved:
			log.Debugf("worker/uniter/filter: want resolved event")
			if f.resolved != state.ResolvedNone {
				f.outResolved = f.outResolvedOn
			}
		}
	}
	panic("unreachable")
}

// unitChanged responds to changes in the unit.
func (f *filter) unitChanged() error {
	if f.life != f.unit.Life() {
		switch f.life = f.unit.Life(); f.life {
		case state.Dying:
			log.Printf("worker/uniter: unit is dying")
			close(f.outUnitDying)
			f.outUpgrade = nil
		case state.Dead:
			log.Printf("worker/uniter: unit is dead")
			return worker.ErrDead
		}
	}
	if resolved := f.unit.Resolved(); resolved != f.resolved {
		f.resolved = resolved
		if f.resolved != state.ResolvedNone {
			f.outResolved = f.outResolvedOn
		}
	}
	return nil
}

// serviceChanged responds to changes in the service.
func (f *filter) serviceChanged() error {
	url, force := f.service.CharmURL()
	f.upgradeAvailable = serviceCharm{url, force}
	switch f.service.Life() {
	case state.Dying:
		if err := f.unit.EnsureDying(); err != nil {
			return err
		}
	case state.Dead:
		return fmt.Errorf("service unexpectedly dead")
	}
	return f.upgradeChanged()
}

// upgradeChanged responds to changes in the service or in the
// upgrade requests that defines which charm changes should be
// delivered as upgrades.
func (f *filter) upgradeChanged() (err error) {
	if f.life != state.Alive {
		log.Debugf("worker/uniter/filter: charm check skipped, unit is dying")
		f.outUpgrade = nil
		return nil
	}
	if *f.upgradeAvailable.url != *f.upgradeRequested.url {
		if f.upgradeAvailable.force || !f.upgradeRequested.force {
			log.Debugf("worker/uniter/filter: preparing new upgrade event")
			if f.upgrade == nil || *f.upgrade.URL() != *f.upgradeAvailable.url {
				if f.upgrade, err = f.st.Charm(f.upgradeAvailable.url); err != nil {
					return err
				}
			}
			f.outUpgrade = f.outUpgradeOn
			return nil
		}
	}
	log.Debugf("worker/uniter/filter: no new charm event")
	return nil
}

// serviceCharm holds information about a charm.
type serviceCharm struct {
	url   *charm.URL
	force bool
}

// nothing is marginally more pleasant to read than "struct{}{}".
var nothing = struct{}{}
