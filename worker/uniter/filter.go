package uniter

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// ErrDead indicates that a uniter's unit is dead, and that the Uniter
// will take no further actions.
var ErrDead = errors.New("unit is dead")

// filter collects unit, service, and service config information from separate
// state watchers, and presents it as events on channels designed specifically
// for the convenience of the uniter.
type filter struct {
	tomb         tomb.Tomb
	outUnitDying chan struct{}
	outResolved  chan *state.ResolvedMode
	outConfig    chan struct{}
	outCharm     chan *charmChange
	wantResolved chan struct{}
	wantConfig   chan struct{}
	wantCharm    chan struct{}
}

// charmChange holds the result of a service's CharmURL method.
type charmChange struct {
	url   *charm.URL
	force bool
}

// newFilter returns a filter that handles state changes pertaining to the
// supplied unit.
func newFilter(unit *state.Unit) *filter {
	f := &filter{
		outUnitDying: make(chan struct{}),
		outResolved:  make(chan *state.ResolvedMode),
		outConfig:    make(chan struct{}),
		outCharm:     make(chan *charmChange),
		wantResolved: make(chan struct{}),
		wantConfig:   make(chan struct{}),
		wantCharm:    make(chan struct{}),
	}
	unitw := unit.Watch()
	go func() {
		defer f.tomb.Done()
		defer watcher.Stop(unitw, &f.tomb)
		err := f.loop(unitw)
		log.Printf("filter error: %v", err)
		f.tomb.Kill(err)
	}()
	return f
}

func (f *filter) loop(unitw *state.UnitWatcher) (err error) {
	// We start off not even knowing what unit we're watching; we determine this
	// from the initial unit change event, and use that to start watchers on the
	// appropriate service and its config.
	// TODO I guess it'll actually have to be unit.WatchConfig(), given the
	// mooted service-config-per-charm-version behaviour.
	var ok bool
	var unit *state.Unit
	var life state.Life
	var service *state.Service
	var configw *state.ConfigWatcher
	var configChanges <-chan *state.ConfigNode
	var servicew *state.ServiceWatcher
	var serviceChanges <-chan *state.Service

	// The out chans are used to send events to the client. They all start out
	// nil, because no event is ready to send; once the relevant initial change
	// event has been handled, they are set to the filter fields with the
	// corresponding names, and are reset to nil when the event has been sent.
	// because we have yet to receive changes from any watcher.
	var outResolved chan<- *state.ResolvedMode
	var outConfig chan<- struct{}
	var outCharm chan<- *charmChange

	// The want chans are used to respond to requests for particular events.
	// They start out nil, and are set to the filter fields with corresponding
	// names, once the first event on the corresponding *Out chan is ready.
	var wantResolved <-chan struct{}
	var wantConfig <-chan struct{}
	var wantCharm <-chan struct{}

	// rm and ch hold the events corresponding to the most recent known states
	// of the unit and the service. The config event, being stateless, does not
	// need to be recorded.
	var rm *state.ResolvedMode
	var ch *charmChange

	log.Printf("watching unit changes")
	for {
		select {
		case <-f.tomb.Dying():
			return tomb.ErrDying
		case unit, ok = <-unitw.Changes():
			log.Debugf("filter: got unit change")
			if !ok {
				return watcher.MustErr(unitw)
			}
			if life != unit.Life() {
				switch life = unit.Life(); life {
				case state.Dying:
					log.Printf("unit is dying")
					close(f.outUnitDying)
				case state.Dead:
					return ErrDead
				}
			}
			rm_ := unit.Resolved()
			if rm != nil && *rm == rm_ {
				continue
			}
			log.Debugf("filter: preparing new resolved event")
			rm = &rm_
			outResolved = f.outResolved
			wantResolved = f.wantResolved

			// If we have not previously done so, we discover the service and
			// start watchers for the service and its config...
			if service == nil {
				if service, err = unit.Service(); err != nil {
					return err
				}
				configw = service.WatchConfig()
				defer watcher.Stop(configw, &f.tomb)
				configChanges = configw.Changes()
				log.Printf("watching config changes")
				servicew = service.Watch()
				defer watcher.Stop(servicew, &f.tomb)
				serviceChanges = servicew.Changes()
				log.Printf("watching service changes")
			}

		// ...and handle the config and service changes as follows.
		case _, ok := <-configChanges:
			log.Debugf("filter got config change")
			if !ok {
				return watcher.MustErr(configw)
			}
			log.Debugf("filter: preparing new config event")
			outConfig = f.outConfig
			wantConfig = f.wantConfig
		case service, ok = <-serviceChanges:
			log.Debugf("filter: got service change")
			if !ok {
				return watcher.MustErr(servicew)
			}
			switch service.Life() {
			case state.Dying:
				log.Debugf("filter: detected service dying")
				if err := unit.EnsureDying(); err != nil {
					return err
				}
			case state.Dead:
				return fmt.Errorf("service died unexpectedly")
			}
			url, force := service.CharmURL()
			if ch != nil && *url == *ch.url && force == ch.force {
				continue
			}
			log.Debugf("filter: preparing new charm event")
			ch = &charmChange{url, force}
			outCharm = f.outCharm
			wantCharm = f.wantCharm

		// Send events on active out chans.
		case outResolved <- rm:
			log.Debugf("filter: sent resolved event")
			outResolved = nil
		case outConfig <- tick:
			log.Debugf("filter: sent config event")
			outConfig = nil
		case outCharm <- ch:
			log.Debugf("filter: sent charm event")
			outCharm = nil

		// On request, reactivate an out chan to ensure an event will be sent.
		case <-wantResolved:
			log.Debugf("filter: ensuring resolved event")
			outResolved = f.outResolved
		case <-wantConfig:
			log.Debugf("filter: ensuring config event")
			outConfig = f.outConfig
		case <-wantCharm:
			log.Debugf("filter: ensuring charm event")
			outCharm = f.outCharm
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
func (f *filter) resolvedEvents() <-chan *state.ResolvedMode {
	return f.outResolved
}

// configEvents returns a channel that will receive a signal whenever the service's
// configuration changes, or when an event is explicitly requested.
func (f *filter) configEvents() <-chan struct{} {
	return f.outConfig
}

// charmEvents returns a channel that will receive a charmChange whenever the
// service's charm changes, or when an event is explicitly requested.
func (f *filter) charmEvents() <-chan *charmChange {
	return f.outCharm
}

// wantResolvedEvent indicates that the filter should send a resolved event.
func (f *filter) wantResolvedEvent() {
	select {
	case <-f.tomb.Dying():
	case f.wantResolved <- tick:
	}
}

// wantConfigEvent indicates that the filter should send a config event.
func (f *filter) wantConfigEvent() {
	select {
	case <-f.tomb.Dying():
	case f.wantConfig <- tick:
	}
}

// wantCharmEvent indicates that the filter should send a charm event.
func (f *filter) wantCharmEvent() {
	select {
	case <-f.tomb.Dying():
	case f.wantCharm <- tick:
	}
}

// tick is marginally more pleasant to read than "struct{}{}".
var tick = struct{}{}
