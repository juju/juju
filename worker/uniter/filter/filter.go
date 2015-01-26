// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filter

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/uniter"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/hook"
)

var filterLogger = loggo.GetLogger("juju.worker.uniter.filter")

// filter collects unit, service, and service config information from separate
// state watchers, and presents it as events on channels designed specifically
// for the convenience of the uniter.
type filter struct {
	st   *uniter.State
	tomb tomb.Tomb

	// outUnitDying is closed when the unit's life becomes Dying.
	outUnitDying chan struct{}

	// The out*On chans are used to deliver events to clients.
	// The out* chans, when set to the corresponding out*On chan (rather than
	// nil) indicate that an event of the appropriate type is ready to send
	// to the client.
	outConfig        chan struct{}
	outConfigOn      chan struct{}
	outAction        chan *hook.Info
	outActionOn      chan *hook.Info
	outUpgrade       chan *charm.URL
	outUpgradeOn     chan *charm.URL
	outResolved      chan params.ResolvedMode
	outResolvedOn    chan params.ResolvedMode
	outRelations     chan []int
	outRelationsOn   chan []int
	outMeterStatus   chan struct{}
	outMeterStatusOn chan struct{}
	outStorage       chan []string
	outStorageOn     chan []string
	// The want* chans are used to indicate that the filter should send
	// events if it has them available.
	wantForcedUpgrade chan bool
	wantResolved      chan struct{}

	// discardConfig is used to indicate that any pending config event
	// should be discarded.
	discardConfig chan struct{}

	// setCharm is used to request that the unit's charm URL be set to
	// a new value. This must be done in the filter's goroutine, so
	// that config watches can be stopped and restarted pointing to
	// the new charm URL. If we don't stop the watch before the
	// (potentially) last reference to that settings document is
	// removed, we'll see spurious errors (and even in the best case,
	// we risk getting notifications for the wrong settings version).
	setCharm chan *charm.URL

	// didSetCharm is used to report back after setting a charm URL.
	didSetCharm chan struct{}

	// clearResolved is used to request that the unit's resolved flag
	// be cleared. This must be done on the filter's goroutine so that
	// it can immediately trigger the unit change handler, and thus
	// ensure that subsquent requests for resolved events -- that land
	// before the next watcher update for the unit -- do not erroneously
	// send out stale values.
	clearResolved chan struct{}

	// didClearResolved is used to report back after clearing the resolved
	// flag.
	didClearResolved chan struct{}

	// The following fields hold state that is collected while running,
	// and used to detect interesting changes to express as events.
	unit             *uniter.Unit
	life             params.Life
	resolved         params.ResolvedMode
	service          *uniter.Service
	upgradeFrom      serviceCharm
	upgradeAvailable serviceCharm
	upgrade          *charm.URL
	relations        []int
	actionsPending   []string
	nextAction       *hook.Info
	storageInstances []string

	// meterStatusCode and meterStatusInfo reflect the meter status values of the unit.
	meterStatusCode string
	meterStatusInfo string
}

// NewFilter returns a filter that handles state changes pertaining to the
// supplied unit.
func NewFilter(st *uniter.State, unitTag names.UnitTag) (Filter, error) {
	f := &filter{
		st:                st,
		outUnitDying:      make(chan struct{}),
		outConfig:         make(chan struct{}),
		outConfigOn:       make(chan struct{}),
		outAction:         make(chan *hook.Info),
		outActionOn:       make(chan *hook.Info),
		outUpgrade:        make(chan *charm.URL),
		outUpgradeOn:      make(chan *charm.URL),
		outResolved:       make(chan params.ResolvedMode),
		outResolvedOn:     make(chan params.ResolvedMode),
		outRelations:      make(chan []int),
		outRelationsOn:    make(chan []int),
		outMeterStatus:    make(chan struct{}),
		outMeterStatusOn:  make(chan struct{}),
		outStorage:        make(chan []string),
		outStorageOn:      make(chan []string),
		wantForcedUpgrade: make(chan bool),
		wantResolved:      make(chan struct{}),
		discardConfig:     make(chan struct{}),
		setCharm:          make(chan *charm.URL),
		didSetCharm:       make(chan struct{}),
		clearResolved:     make(chan struct{}),
		didClearResolved:  make(chan struct{}),
	}
	go func() {
		defer f.tomb.Done()
		err := f.loop(unitTag)
		filterLogger.Errorf("%v", err)
		f.tomb.Kill(err)
	}()
	return f, nil
}

func (f *filter) Stop() error {
	f.tomb.Kill(nil)
	return f.tomb.Wait()
}

func (f *filter) Dead() <-chan struct{} {
	return f.tomb.Dead()
}

func (f *filter) Wait() error {
	return f.tomb.Wait()
}

// UnitDying returns a channel which is closed when the Unit enters a Dying state.
func (f *filter) UnitDying() <-chan struct{} {
	return f.outUnitDying
}

// UpgradeEvents returns a channel that will receive a new charm URL whenever an
// upgrade is indicated. Events should not be read until the baseline state
// has been specified by calling WantUpgradeEvent.
func (f *filter) UpgradeEvents() <-chan *charm.URL {
	return f.outUpgradeOn
}

// ResolvedEvents returns a channel that may receive a ResolvedMode when the
// unit's Resolved value changes, or when an event is explicitly requested.
// A ResolvedNone state will never generate events, but ResolvedRetryHooks and
// ResolvedNoHooks will always be delivered as described.
func (f *filter) ResolvedEvents() <-chan params.ResolvedMode {
	return f.outResolvedOn
}

// MeterStatusEvents returns a channel that will receive a signal when the unit's
// meter status changes.
func (f *filter) MeterStatusEvents() <-chan struct{} {
	return f.outMeterStatusOn
}

// ConfigEvents returns a channel that will receive a signal whenever the service's
// configuration changes, or when an event is explicitly requested.
func (f *filter) ConfigEvents() <-chan struct{} {
	return f.outConfigOn
}

// ActionEvents returns a channel that will receive a signal whenever the unit
// receives new Actions.
func (f *filter) ActionEvents() <-chan *hook.Info {
	return f.outActionOn
}

// RelationsEvents returns a channel that will receive the ids of all the service's
// relations whose Life status has changed.
func (f *filter) RelationsEvents() <-chan []int {
	return f.outRelationsOn
}

// StorageEvents returns a channel that will receive the ids of the unit's storage
// instances when they change.
func (f *filter) StorageEvents() <-chan []string {
	return f.outStorageOn
}

// WantUpgradeEvent controls whether the filter will generate upgrade
// events for unforced service charm changes.
func (f *filter) WantUpgradeEvent(mustForce bool) {
	select {
	case <-f.tomb.Dying():
	case f.wantForcedUpgrade <- mustForce:
	}
}

// SetCharm notifies the filter that the unit is running a new
// charm. It causes the unit's charm URL to be set in state, and the
// following changes to the filter's behaviour:
//
// * Upgrade events will only be generated for charms different to
//   that supplied;
// * A fresh relations event will be generated containing every relation
//   the service is participating in;
// * A fresh configuration event will be generated, and subsequent
//   events will only be sent in response to changes in the version
//   of the service's settings that is specific to that charm.
//
// SetCharm blocks until the charm URL is set in state, returning any
// error that occurred.
func (f *filter) SetCharm(curl *charm.URL) error {
	select {
	case <-f.tomb.Dying():
		return tomb.ErrDying
	case f.setCharm <- curl:
	}
	select {
	case <-f.tomb.Dying():
		return tomb.ErrDying
	case <-f.didSetCharm:
		return nil
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

// ClearResolved notifies the filter that a resolved event has been handled
// and should not be reported again.
func (f *filter) ClearResolved() error {
	select {
	case <-f.tomb.Dying():
		return tomb.ErrDying
	case f.clearResolved <- nothing:
	}
	select {
	case <-f.tomb.Dying():
		return tomb.ErrDying
	case <-f.didClearResolved:
		filterLogger.Debugf("resolved clear completed")
		return nil
	}
}

// DiscardConfigEvent indicates that the filter should discard any pending
// config event.
func (f *filter) DiscardConfigEvent() {
	select {
	case <-f.tomb.Dying():
	case f.discardConfig <- nothing:
	}
}

func (f *filter) maybeStopWatcher(w watcher.Stopper) {
	if w != nil {
		watcher.Stop(w, &f.tomb)
	}
}

func (f *filter) loop(unitTag names.UnitTag) (err error) {
	// TODO(dfc) named return value is a time bomb
	defer func() {
		if params.IsCodeNotFoundOrCodeUnauthorized(err) {
			err = worker.ErrTerminateAgent
		}
	}()
	if f.unit, err = f.st.Unit(unitTag); err != nil {
		return err
	}
	if err = f.unitChanged(); err != nil {
		return err
	}
	if err = f.meterStatusChanged(); err != nil {
		return err
	}
	f.service, err = f.unit.Service()
	if err != nil {
		return err
	}
	if err = f.serviceChanged(); err != nil {
		return err
	}
	unitw, err := f.unit.Watch()
	if err != nil {
		return err
	}
	defer f.maybeStopWatcher(unitw)
	servicew, err := f.service.Watch()
	if err != nil {
		return err
	}
	defer f.maybeStopWatcher(servicew)
	// configw and relationsw can get restarted, so we need to use
	// their eventual values in the defer calls.
	var configw apiwatcher.NotifyWatcher
	var configChanges <-chan struct{}
	curl, err := f.unit.CharmURL()
	if err == nil {
		configw, err = f.unit.WatchConfigSettings()
		if err != nil {
			return err
		}
		configChanges = configw.Changes()
		f.upgradeFrom.url = curl
	} else if err != uniter.ErrNoCharmURLSet {
		filterLogger.Errorf("unit charm: %v", err)
		return err
	}
	defer f.maybeStopWatcher(configw)
	actionsw, err := f.unit.WatchActionNotifications()
	if err != nil {
		return err
	}
	f.actionsPending = make([]string, 0)
	defer f.maybeStopWatcher(actionsw)
	relationsw, err := f.service.WatchRelations()
	if err != nil {
		return err
	}
	defer f.maybeStopWatcher(relationsw)
	meterStatusw, err := f.unit.WatchMeterStatus()
	if err != nil {
		return err
	}
	defer f.maybeStopWatcher(meterStatusw)
	addressesw, err := f.unit.WatchAddresses()
	if err != nil {
		return err
	}
	defer watcher.Stop(addressesw, &f.tomb)
	storagew, err := f.unit.WatchStorageInstances()
	if err != nil {
		return err
	}
	defer watcher.Stop(storagew, &f.tomb)

	// Config events cannot be meaningfully discarded until one is available;
	// once we receive the initial config and address changes, we unblock
	// discard requests by setting this channel to its namesake on f.
	var discardConfig chan struct{}
	var seenConfigChange bool
	var seenAddressChange bool
	maybePrepareConfigEvent := func() {
		if !seenAddressChange {
			filterLogger.Debugf("no address change seen yet, skipping config event")
			return
		}
		if !seenConfigChange {
			filterLogger.Debugf("no config change seen yet, skipping config event")
			return
		}
		filterLogger.Debugf("preparing new config event")
		f.outConfig = f.outConfigOn
		discardConfig = f.discardConfig
	}

	for {
		var ok bool
		select {
		case <-f.tomb.Dying():
			return tomb.ErrDying

		// Handle watcher changes.
		case _, ok = <-unitw.Changes():
			filterLogger.Debugf("got unit change")
			if !ok {
				return watcher.EnsureErr(unitw)
			}
			if err = f.unitChanged(); err != nil {
				return err
			}
		case _, ok = <-servicew.Changes():
			filterLogger.Debugf("got service change")
			if !ok {
				return watcher.EnsureErr(servicew)
			}
			if err = f.serviceChanged(); err != nil {
				return err
			}
		case _, ok = <-configChanges:
			filterLogger.Debugf("got config change")
			if !ok {
				return watcher.EnsureErr(configw)
			}
			seenConfigChange = true
			maybePrepareConfigEvent()
		case _, ok = <-addressesw.Changes():
			filterLogger.Debugf("got address change")
			if !ok {
				return watcher.EnsureErr(addressesw)
			}
			seenAddressChange = true
			maybePrepareConfigEvent()
		case _, ok = <-meterStatusw.Changes():
			filterLogger.Debugf("got meter status change")
			if !ok {
				return watcher.EnsureErr(configw)
			}
			if err = f.meterStatusChanged(); err != nil {
				return errors.Trace(err)
			}
		case ids, ok := <-actionsw.Changes():
			filterLogger.Debugf("got %d actions", len(ids))
			if !ok {
				return watcher.EnsureErr(actionsw)
			}
			f.actionsPending = append(f.actionsPending, ids...)
			f.nextAction = f.getNextAction()
		case keys, ok := <-relationsw.Changes():
			filterLogger.Debugf("got relations change")
			if !ok {
				return watcher.EnsureErr(relationsw)
			}
			var ids []int
			for _, key := range keys {
				relationTag := names.NewRelationTag(key)
				rel, err := f.st.Relation(relationTag)
				if params.IsCodeNotFoundOrCodeUnauthorized(err) {
					// If it's actually gone, this unit cannot have entered
					// scope, and therefore never needs to know about it.
				} else if err != nil {
					return err
				} else {
					ids = append(ids, rel.Id())
				}
			}
			f.relationsChanged(ids)
		case ids, ok := <-storagew.Changes():
			filterLogger.Debugf("got storage change")
			if !ok {
				return watcher.EnsureErr(storagew)
			}
			f.storageChanged(ids)

		// Send events on active out chans.
		case f.outUpgrade <- f.upgrade:
			filterLogger.Debugf("sent upgrade event")
			f.outUpgrade = nil
		case f.outResolved <- f.resolved:
			filterLogger.Debugf("sent resolved event")
			f.outResolved = nil
		case f.outConfig <- nothing:
			filterLogger.Debugf("sent config event")
			f.outConfig = nil
		case f.outAction <- f.nextAction:
			f.nextAction = f.getNextAction()
			filterLogger.Debugf("sent action event")
		case f.outRelations <- f.relations:
			filterLogger.Debugf("sent relations event")
			f.outRelations = nil
			f.relations = nil
		case f.outMeterStatus <- nothing:
			filterLogger.Debugf("sent meter status change event")
			f.outMeterStatus = nil
		case f.outStorage <- f.storageInstances:
			filterLogger.Debugf("sent storage event")
			f.outStorage = nil
			f.storageInstances = nil

		// Handle explicit requests.
		case curl := <-f.setCharm:
			filterLogger.Debugf("changing charm to %q", curl)
			// We need to restart the config watcher after setting the
			// charm, because service config settings are distinct for
			// different service charms.
			if configw != nil {
				if err := configw.Stop(); err != nil {
					return err
				}
			}
			if err := f.unit.SetCharmURL(curl); err != nil {
				filterLogger.Debugf("failed setting charm url %q: %v", curl, err)
				return err
			}
			select {
			case <-f.tomb.Dying():
				return tomb.ErrDying
			case f.didSetCharm <- nothing:
			}
			configw, err = f.unit.WatchConfigSettings()
			if err != nil {
				return err
			}
			configChanges = configw.Changes()

			// Restart the relations watcher.
			if err := relationsw.Stop(); err != nil {
				return err
			}
			relationsw, err = f.service.WatchRelations()
			if err != nil {
				return err
			}

			f.upgradeFrom.url = curl
			if err = f.upgradeChanged(); err != nil {
				return err
			}
		case force := <-f.wantForcedUpgrade:
			filterLogger.Debugf("want forced upgrade %v", force)
			f.upgradeFrom.force = force
			if err = f.upgradeChanged(); err != nil {
				return err
			}
		case <-f.wantResolved:
			filterLogger.Debugf("want resolved event")
			if f.resolved != params.ResolvedNone {
				f.outResolved = f.outResolvedOn
			}
		case <-f.clearResolved:
			filterLogger.Debugf("resolved event handled")
			f.outResolved = nil
			if err := f.unit.ClearResolved(); err != nil {
				return err
			}
			if err := f.unitChanged(); err != nil {
				return err
			}
			select {
			case <-f.tomb.Dying():
				return tomb.ErrDying
			case f.didClearResolved <- nothing:
			}
		case <-discardConfig:
			filterLogger.Debugf("discarded config event")
			f.outConfig = nil
		}
	}
}

// meterStatusChanges respondes to changes in the unit's meter status.
func (f *filter) meterStatusChanged() error {
	code, info, err := f.unit.MeterStatus()
	if err != nil {
		return errors.Trace(err)
	}
	if f.meterStatusCode != code || f.meterStatusInfo != info {
		if f.meterStatusCode != "" {
			f.outMeterStatus = f.outMeterStatusOn
		}
		f.meterStatusCode = code
		f.meterStatusInfo = info
	}
	return nil
}

// unitChanged responds to changes in the unit.
func (f *filter) unitChanged() error {
	if err := f.unit.Refresh(); err != nil {
		return err
	}
	if f.life != f.unit.Life() {
		switch f.life = f.unit.Life(); f.life {
		case params.Dying:
			filterLogger.Infof("unit is dying")
			close(f.outUnitDying)
			f.outUpgrade = nil
		case params.Dead:
			filterLogger.Infof("unit is dead")
			return worker.ErrTerminateAgent
		}
	}
	resolved, err := f.unit.Resolved()
	if err != nil {
		return err
	}
	if resolved != f.resolved {
		f.resolved = resolved
		if f.resolved != params.ResolvedNone {
			f.outResolved = f.outResolvedOn
		}
	}
	return nil
}

// serviceChanged responds to changes in the service.
func (f *filter) serviceChanged() error {
	if err := f.service.Refresh(); err != nil {
		return err
	}
	url, force, err := f.service.CharmURL()
	if err != nil {
		return err
	}
	f.upgradeAvailable = serviceCharm{url, force}
	switch f.service.Life() {
	case params.Dying:
		if err := f.unit.Destroy(); err != nil {
			return err
		}
	case params.Dead:
		filterLogger.Infof("service is dead")
		return worker.ErrTerminateAgent
	}
	return f.upgradeChanged()
}

// upgradeChanged responds to changes in the service or in the
// upgrade requests that defines which charm changes should be
// delivered as upgrades.
func (f *filter) upgradeChanged() (err error) {
	if f.life != params.Alive {
		filterLogger.Debugf("charm check skipped, unit is dying")
		f.outUpgrade = nil
		return nil
	}
	if f.upgradeFrom.url == nil {
		filterLogger.Debugf("charm check skipped, not yet installed.")
		f.outUpgrade = nil
		return nil
	}
	if *f.upgradeAvailable.url != *f.upgradeFrom.url {
		if f.upgradeAvailable.force || !f.upgradeFrom.force {
			filterLogger.Debugf("preparing new upgrade event")
			if f.upgrade == nil || *f.upgrade != *f.upgradeAvailable.url {
				f.upgrade = f.upgradeAvailable.url
			}
			f.outUpgrade = f.outUpgradeOn
			return nil
		}
	}
	filterLogger.Debugf("no new charm event")
	f.outUpgrade = nil
	return nil
}

// relationsChanged responds to service relation changes.
func (f *filter) relationsChanged(ids []int) {
outer:
	for _, id := range ids {
		for _, existing := range f.relations {
			if id == existing {
				continue outer
			}
		}
		f.relations = append(f.relations, id)
	}
	if len(f.relations) != 0 {
		sort.Ints(f.relations)
		f.outRelations = f.outRelationsOn
	}
}

// storageChanged responds to storage instance changes.
func (f *filter) storageChanged(ids []string) {
outer:
	for _, id := range ids {
		for _, existing := range f.storageInstances {
			if id == existing {
				continue outer
			}
		}
		f.storageInstances = append(f.storageInstances, id)
	}
	if len(f.storageInstances) != 0 {
		sort.Strings(f.storageInstances)
		f.outStorage = f.outStorageOn
	}
}

func (f *filter) getNextAction() *hook.Info {
	if len(f.actionsPending) > 0 {
		nextAction := hook.Info{
			Kind:     hooks.Action,
			ActionId: f.actionsPending[0],
		}

		f.outAction = f.outActionOn
		f.actionsPending = f.actionsPending[1:]

		return &nextAction
	} else {
		f.outAction = nil
	}

	return nil
}

// serviceCharm holds information about a charm.
type serviceCharm struct {
	url   *charm.URL
	force bool
}

// nothing is marginally more pleasant to read than "struct{}{}".
var nothing = struct{}{}
