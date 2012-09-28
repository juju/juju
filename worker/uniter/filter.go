package uniter

import (
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

// filter converts changes received from state watchers into events on channels
// designed for the convenience of the Uniter.
type filter struct {
	tombDyingChan    <-chan struct{}
	unitDyingChan    chan struct{}
	resolvedChan     chan *state.ResolvedMode
	configChan       chan struct{}
	charmChan        chan *charmChange
	wantResolvedChan chan struct{}
	wantConfigChan   chan struct{}
	wantCharmChan    chan struct{}
}

// newFilter returns a filter that will stop when the supplied channel is closed.
func newFilter(tombDyingChan <-chan struct{}) *filter {
	return &filter{
		tombDyingChan:    tombDyingChan,
		unitDyingChan:    make(chan struct{}),
		resolvedChan:     make(chan *state.ResolvedMode),
		configChan:       make(chan struct{}),
		charmChan:        make(chan *charmChange),
		wantResolvedChan: make(chan struct{}),
		wantConfigChan:   make(chan struct{}),
		wantCharmChan:    make(chan struct{}),
	}
}

// charmChange holds the result of a service's CharmURL method.
type charmChange struct {
	URL   *charm.URL
	Force bool
}

func (f *filter) loop(unitw *state.UnitWatcher, servicew *state.ServiceWatcher, configw *state.ConfigWatcher) (err error) {
	// rm and ch hold the values of the current events on the resolved and
	// charm channels.
	var rm *state.ResolvedMode
	var ch *charmChange

	// Before receiving the initial changes, we don't want to send any events...
	var resolvedChan chan<- *state.ResolvedMode
	var configChan chan<- struct{}
	var charmChan chan<- *charmChange

	// ...and we can't handle requests until we have something to send, either.
	var wantResolvedChan <-chan struct{}
	var wantConfigChan <-chan struct{}
	var wantCharmChan <-chan struct{}

	// We also start off with nil channels for service/config changes, so that
	// we can determine unit state before sending any other events that could
	// be misinterpreted due to inaccurate life state (and, also, so we can
	// make use of the unit in the service change handler).
	var ok bool
	var unit *state.Unit
	var configChanges <-chan *state.ConfigNode
	var serviceChanges <-chan *state.Service

	log.Printf("filtering unit changes")
	for {
		select {
		// If the control channel closes, bail out immediately.
		case <-f.tombDyingChan:
			return tomb.ErrDying

		// Always handle changes from the unit watcher.
		case unit, ok = <-unitw.Changes():
			log.Debugf("filter: got unit change")
			if !ok {
				return watcher.MustErr(unitw)
			}
			switch unit.Life() {
			case state.Dying:
				log.Printf("unit is dying")
				close(f.unitDyingChan)
			case state.Dead:
				return ErrDead
			}
			rm_ := unit.Resolved()
			if rm == nil {
				configChanges = configw.Changes()
				serviceChanges = servicew.Changes()
				log.Printf("filtering all changes")
			} else if rm_ == *rm {
				continue
			}
			log.Debugf("filter: preparing new resolved event")
			rm = &rm_
			resolvedChan = f.resolvedChan
			wantResolvedChan = f.wantResolvedChan

		// Once we've handled any unit change, always watch for config changes.
		case _, ok := <-configChanges:
			log.Debugf("filter got config change")
			if !ok {
				return watcher.MustErr(configw)
			}
			log.Debugf("filter: preparing new config event")
			configChan = f.configChan
			wantConfigChan = f.wantConfigChan

		// Once we've handled any unit change, always watch for service changes.
		case service, ok := <-serviceChanges:
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
			charmChan = f.charmChan
			wantCharmChan = f.wantCharmChan

		// Send any events that are available.
		case resolvedChan <- rm:
			log.Debugf("filter: sent resolved event")
			resolvedChan = nil
		case configChan <- tick:
			log.Debugf("filter: sent config event")
			configChan = nil
		case charmChan <- ch:
			log.Debugf("filter: sent charm event")
			charmChan = nil

		// On request, if no event is ready to send, reuse the last event.
		case <-wantResolvedChan:
			log.Debugf("filter: refreshing resolved event")
			resolvedChan = f.resolvedChan
		case <-wantConfigChan:
			log.Debugf("filter: refreshing config event")
			configChan = f.configChan
		case <-wantCharmChan:
			log.Debugf("filter: refreshing charm event")
			charmChan = f.charmChan
		}
	}
	panic("unreachable")
}

// tombDying returns a channel which is closed when the Uniter is being shut down.
func (f *filter) tombDying() <-chan struct{} {
	return f.tombDyingChan
}

// unitDying returns a channel which is closed when the unit enters a Dying state.
func (f *filter) unitDying() <-chan struct{} {
	return f.unitDyingChan
}

// resolvedEvents returns a channel that will receive a ResolvedMode whenever the
// unit's Resolved value changes, or when an event is explicitly requested.
func (f *filter) resolvedEvents() <-chan *state.ResolvedMode {
	return f.resolvedChan
}

// configEvents returns a channel that will receive a signal whenever the service's
// configuration changes, or when an event is explicitly requested.
func (f *filter) configEvents() <-chan struct{} {
	return f.configChan
}

// charmEvents returns a channel that will receive a charmChange whenever the
// service's charm changes, or when an event is explicitly requested.
func (f *filter) charmEvents() <-chan *charmChange {
	return f.charmChan
}

// wantResolvedEvent indicates that the filter should send a resolved event.
// If no more recent event is available, the previous event will be resent.
func (f *filter) wantResolvedEvent() {
	select {
	case <-f.tombDyingChan:
	case f.wantResolvedChan <- tick:
	}
}

// wantConfigEvent indicates that the filter should send a config event. If
// no more recent event is available, the previous event will be resent.
func (f *filter) wantConfigEvent() {
	select {
	case <-f.tombDyingChan:
	case f.wantConfigChan <- tick:
	}
}

// wantCharmEvent indicates that the filter should send a charm event. If no
// more recent event is available, the previous event will be resent.
func (f *filter) wantCharmEvent() {
	select {
	case <-f.tombDyingChan:
	case f.wantCharmChan <- tick:
	}
}

// tick is marginally more pleasant to read than "struct{}{}".
var tick = struct{}{}
