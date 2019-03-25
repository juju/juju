// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sort"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/pubsub"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/lxdprofile"
)

// The watchers used in the cache package are closer to state watchers
// than core watchers. The core watchers never close their changes channel,
// which leads to issues in the apiserver facade methods dealing with
// watchers. So the watchers in this package do close their changes channels.

// Watcher is the common methods
type Watcher interface {
	worker.Worker
	// Stop is currently needed by the apiserver until the resources
	// work on workers instead of things that can be stopped.
	Stop() error
}

// NotifyWatcher will only say something changed.
type NotifyWatcher interface {
	Watcher
	Changes() <-chan struct{}
}

type notifyWatcherBase struct {
	tomb    tomb.Tomb
	changes chan struct{}
	// We can't send down a closed channel, so protect the sending
	// with a mutex and bool. Since you can't really even ask a channel
	// if it is closed.
	closed bool
	mu     sync.Mutex
}

func newNotifyWatcherBase() *notifyWatcherBase {
	// We use a single entry buffered channel for the changes.
	// This allows the config changed handler to send a value when there
	// is a change, but if that value hasn't been consumed before the
	// next change, the second change is discarded.
	ch := make(chan struct{}, 1)

	// Send initial event down the channel. We know that this will
	// execute immediately because it is a buffered channel.
	ch <- struct{}{}

	return &notifyWatcherBase{changes: ch}
}

// Changes is part of the core watcher definition.
// The changes channel is never closed.
func (w *notifyWatcherBase) Changes() <-chan struct{} {
	return w.changes
}

// Kill is part of the worker.Worker interface.
func (w *notifyWatcherBase) Kill() {
	w.mu.Lock()
	w.closed = true
	close(w.changes)
	w.mu.Unlock()
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *notifyWatcherBase) Wait() error {
	return w.tomb.Wait()
}

// Stop is currently required by the Resources wrapper in the apiserver.
func (w *notifyWatcherBase) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *notifyWatcherBase) notify() {
	w.mu.Lock()

	if w.closed {
		w.mu.Unlock()
		return
	}

	select {
	case w.changes <- struct{}{}:
	default:
		// Already a pending change, so do nothing.
	}

	w.mu.Unlock()
}

// ConfigWatcher watches a single entity's configuration.
// If keys are specified the watcher only signals a change when at least one
// of those keys changes value. If no keys are specified,
// any change in the config will trigger the watcher to notify.
type ConfigWatcher struct {
	*notifyWatcherBase

	keys []string
	hash string
}

// newConfigWatcher returns a new watcher for the input config keys
// with a baseline hash of their config values from the input hash cache.
// As per the cache requirements, hashes are only generated from sorted keys.
func newConfigWatcher(keys []string, cache *hashCache, hub *pubsub.SimpleHub, topic string) *ConfigWatcher {
	sort.Strings(keys)

	w := &ConfigWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),

		keys: keys,
		hash: cache.getHash(keys),
	}

	unsub := hub.Subscribe(topic, w.configChanged)
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		return nil
	})

	return w
}

func (w *ConfigWatcher) configChanged(topic string, value interface{}) {
	hashCache, ok := value.(*hashCache)
	if !ok {
		logger.Errorf("programming error, value not of type *hashCache")
	}
	hash := hashCache.getHash(w.keys)
	if hash == w.hash {
		// Nothing that we care about has changed, so we're done.
		return
	}
	w.notify()
}

type MachineAppLXDProfileWatcher struct {
	*notifyWatcherBase

	applications map[string]appInfo // unit names for each application
	machineId    string

	getCharm charmFunc
}

type charmFunc func(string) (*Charm, error)

type appInfo struct {
	charmURL     string
	charmProfile *lxdprofile.Profile
	units        set.Strings
}

func newMachineAppLXDProfileWatcher(
	appTopic, unitTopic, machineId string,
	applications map[string]appInfo,
	getCharm charmFunc,
	hub *pubsub.SimpleHub,
) *MachineAppLXDProfileWatcher {
	w := &MachineAppLXDProfileWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),
		applications:      applications,
		getCharm:          getCharm,
		machineId:         machineId,
	}

	unsubApp := hub.Subscribe(appTopic, w.applicationCharmURLChange)
	unsubUnit := hub.Subscribe(unitTopic, w.unitChange)
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsubApp()
		unsubUnit()
		return nil
	})

	return w
}

// applicationCharmURLChange sends a notification if what is saved for its
// charm lxdprofile changes.  No notification is sent if the pointer begins
// and ends as nil.
func (w *MachineAppLXDProfileWatcher) applicationCharmURLChange(topic string, value interface{}) {
	w.notifyWatcherBase.mu.Lock()
	var notify bool
	defer func(notify *bool) {
		w.notifyWatcherBase.mu.Unlock()
		if *notify {
			w.notify()
		}
	}(&notify)

	values, ok := value.([]string)
	if !ok {
		logger.Errorf("programming error, value not of type []string")
		return
	}
	if len(values) != 2 {
		logger.Errorf("programming error, 2 values not provided")
		return
	}
	appName, chURL := values[0], values[1]
	info, ok := w.applications[appName]
	if ok {
		ch, err := w.getCharm(chURL)
		if err != nil {

		}
		// notify if:
		// 1. the prior charm had a profile and the new one does not.
		// 2. the new profile is not empty.
		if (info.charmProfile != nil && ch.details.LXDProfile.Empty()) ||
			!ch.details.LXDProfile.Empty() {
			logger.Tracef("notifying due to change of charm lxd profile for %s, machine-%s", appName, w.machineId)
			notify = true
		} else {
			logger.Tracef("no notification of charm lxd profile needed for %s, machine-%s", appName, w.machineId)
		}
		if ch.details.LXDProfile.Empty() {
			info.charmProfile = nil
		} else {
			info.charmProfile = &ch.details.LXDProfile
		}
		info.charmURL = chURL
		w.applications[appName] = info
	} else {
		logger.Errorf("not watching %s on machine-%s", appName, w.machineId)
	}
}

// unitChange modifies the map of applications being watched when a unit is
// added or removed from the machine.  Notification is sent if:
//     1. A new unit whose charm has an lxd profile is added.
//     2. A unit being removed has a profile and other units
//        exist on the machine.
func (w *MachineAppLXDProfileWatcher) unitChange(topic string, value interface{}) {
	w.notifyWatcherBase.mu.Lock()
	var notify bool
	defer func(notify *bool) {
		w.notifyWatcherBase.mu.Unlock()
		if *notify {
			logger.Tracef("notifying due to add/remove unit requires lxd profile change")
			w.notify()
		}
	}(&notify)

	names, okString := value.([]string)
	unit, okUnit := value.(*Unit)
	switch {
	case okString:
		logger.Tracef("Stop watching %q", names)
		notify = w.removeUnit(names)
	case okUnit:
		if w.machineId != unit.details.MachineId {
			logger.Tracef("not the machine being watched")
			return
		}
		logger.Tracef("Start watching %q", unit.details.Name)
		notify = w.addUnit(unit)
	default:
		logger.Errorf("programming error, value not of type *Unit or []string")
	}
}

func (w *MachineAppLXDProfileWatcher) addUnit(unit *Unit) bool {
	_, ok := w.applications[unit.details.Application]
	if !ok {
		info := appInfo{
			charmURL: unit.details.CharmURL,
			units:    set.NewStrings(unit.details.Name),
		}
		ch, err := w.getCharm(unit.details.CharmURL)
		if err != nil {

		}
		if !ch.details.LXDProfile.Empty() {
			info.charmProfile = &ch.details.LXDProfile
		}
		w.applications[unit.details.Application] = info
	} else {
		w.applications[unit.details.Application].units.Add(unit.details.Name)
	}
	if w.applications[unit.details.Application].charmProfile != nil {
		return true
	}
	return false
}

func (w *MachineAppLXDProfileWatcher) removeUnit(names []string) bool {
	if len(names) != 2 {
		logger.Errorf("programming error, 2 values not provided")
		return false
	}
	unitName, appName := names[0], names[1]
	_, ok := w.applications[appName]
	if !ok {
		logger.Errorf("programming error, unit removed before being added, application name not found")
		return false
	}
	if !w.applications[appName].units.Contains(unitName) {
		logger.Errorf("unit not being watched for machine")
		return false
	}
	profile := w.applications[appName].charmProfile
	w.applications[appName].units.Remove(unitName)
	if w.applications[appName].units.Size() == 0 {
		// the application has no more units on this machine,
		// stop watching it.
		delete(w.applications, appName)
	}
	// If there are additional units on the machine and the current
	// application has an lxd profile, notify so it can be removed
	// from the machine.
	if len(w.applications) > 0 && profile != nil && !profile.Empty() {
		return true
	}
	return false
}

// StringsWatcher will return what has changed.
type StringsWatcher interface {
	Watcher
	Changes() <-chan []string
}

type stringsWatcherBase struct {
	tomb    tomb.Tomb
	changes chan []string
	// We can't send down a closed channel, so protect the sending
	// with a mutex and bool. Since you can't really even ask a channel
	// if it is closed.
	closed bool
	mu     sync.Mutex
}

func newStringsWatcherBase(values ...string) *stringsWatcherBase {
	// We use a single entry buffered channel for the changes.
	// This allows the config changed handler to send a value when there
	// is a change, if that value hasn't been consumed before the
	// next change, the changes are combined.
	ch := make(chan []string, 1)

	// Send initial event down the channel. We know that this will
	// execute immediately because it is a buffered channel.
	ch <- values

	return &stringsWatcherBase{changes: ch}
}

// Changes is part of the core watcher definition.
// The changes channel is never closed.
func (w *stringsWatcherBase) Changes() <-chan []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.changes
}

// Kill is part of the worker.Worker interface.
func (w *stringsWatcherBase) Kill() {
	w.mu.Lock()
	w.closed = true
	close(w.changes)
	w.mu.Unlock()
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *stringsWatcherBase) Wait() error {
	return w.tomb.Wait()
}

// Stop is currently required by the Resources wrapper in the apiserver.
func (w *stringsWatcherBase) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *stringsWatcherBase) notify(values []string) {
	w.mu.Lock()

	if w.closed {
		w.mu.Unlock()
		return
	}

	select {
	case w.changes <- values:
	default:
		// Already a pending change, so add the new values to the
		// pending change.
		w.amendBufferedChange(values)
	}

	w.mu.Unlock()
}

// amendBufferedChange alters the buffered notification to include new
// information. This method assumes lock protection.
func (w *stringsWatcherBase) amendBufferedChange(values []string) {
	select {
	case old := <-w.changes:
		w.changes <- set.NewStrings(old...).Union(set.NewStrings(values...)).Values()
	default:
		// Someone read the channel in the meantime.
		// We know we're locked, so no further writes will have occurred.
		// Just send what we were going to send.
		w.changes <- values
	}
}

// ChangeWatcher notifies that something changed, with
// the given slice of strings.  An initial event is sent
// with the input given at creation.
type ChangeWatcher struct {
	*stringsWatcherBase
}

func newAddRemoveWatcher(values ...string) *ChangeWatcher {
	return &ChangeWatcher{
		stringsWatcherBase: newStringsWatcherBase(values...),
	}
}

func (w *ChangeWatcher) changed(topic string, value interface{}) {
	strings, ok := value.([]string)
	if !ok {
		logger.Errorf("programming error, value not of type []string")
	}

	w.notify(strings)
}
