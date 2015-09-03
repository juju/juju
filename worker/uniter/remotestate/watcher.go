// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/leadership"
)

var logger = loggo.GetLogger("juju.worker.uniter.remotestate")

// RemoteStateWatcher collects unit, service, and service config information
// from separate state watchers, and updates a Snapshot which is sent on a
// channel upon change.
type RemoteStateWatcher struct {
	st                        State
	unit                      Unit
	service                   Service
	relations                 map[names.RelationTag]*relationUnitsWatcher
	relationUnitsChanges      chan relationUnitsChange
	storageAttachmentWatchers map[names.StorageTag]*storageAttachmentWatcher
	storageAttachmentChanges  chan storageAttachmentChange
	leadershipTracker         leadership.Tracker
	updateStatusChannel       func() <-chan time.Time

	tomb tomb.Tomb

	out     chan struct{}
	mu      sync.Mutex
	current Snapshot
}

// WatcherConfig holds configuration parameters for the
// remote state watcher.
type WatcherConfig struct {
	State               State
	LeadershipTracker   leadership.Tracker
	UpdateStatusChannel func() <-chan time.Time
	UnitTag             names.UnitTag
}

// TimedSignal is the signature of a function used to generate a
// hook signal.
type TimedSignal func(now, lastSignal time.Time, interval time.Duration) <-chan time.Time

// NewWatcher returns a RemoteStateWatcher that handles state changes pertaining to the
// supplied unit.
func NewWatcher(config WatcherConfig) (*RemoteStateWatcher, error) {
	w := &RemoteStateWatcher{
		st:                        config.State,
		relations:                 make(map[names.RelationTag]*relationUnitsWatcher),
		relationUnitsChanges:      make(chan relationUnitsChange),
		storageAttachmentWatchers: make(map[names.StorageTag]*storageAttachmentWatcher),
		storageAttachmentChanges:  make(chan storageAttachmentChange),
		leadershipTracker:         config.LeadershipTracker,
		updateStatusChannel:       config.UpdateStatusChannel,
		// Note: it is important that the out channel be buffered!
		// The remote state watcher will perform a non-blocking send
		// on the channel to wake up the observer. It is non-blocking
		// so that we coalesce events while the observer is busy.
		out: make(chan struct{}, 1),
		current: Snapshot{
			Relations: make(map[int]RelationSnapshot),
			Storage:   make(map[names.StorageTag]StorageSnapshot),
		},
	}
	if err := w.init(config.UnitTag); err != nil {
		return nil, errors.Trace(err)
	}
	go func() {
		defer w.tomb.Done()
		err := w.loop(config.UnitTag)
		logger.Errorf("remote state watcher exited: %v", err)
		w.tomb.Kill(errors.Cause(err))
	}()
	return w, nil
}

func (w *RemoteStateWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *RemoteStateWatcher) Dead() <-chan struct{} {
	return w.tomb.Dead()
}

func (w *RemoteStateWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *RemoteStateWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *RemoteStateWatcher) RemoteStateChanged() <-chan struct{} {
	return w.out
}

func (w *RemoteStateWatcher) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	snapshot := w.current
	snapshot.Relations = make(map[int]RelationSnapshot)
	for id, relationSnapshot := range w.current.Relations {
		snapshot.Relations[id] = relationSnapshot
	}
	snapshot.Storage = make(map[names.StorageTag]StorageSnapshot)
	for tag, storageSnapshot := range w.current.Storage {
		snapshot.Storage[tag] = storageSnapshot
	}
	snapshot.Actions = make([]string, len(w.current.Actions))
	for i, action := range w.current.Actions {
		snapshot.Actions[i] = action
	}
	// We return a snapshot with the current UpdateStatusRequired value.
	// We reset it so that subsequent snapshots wait until the timer is
	// triggered again before setting the value again.
	w.current.UpdateStatusRequired = false
	return snapshot
}

func (w *RemoteStateWatcher) ClearResolvedMode() {
	w.mu.Lock()
	w.current.ResolvedMode = params.ResolvedNone
	w.mu.Unlock()
}

func (w *RemoteStateWatcher) init(unitTag names.UnitTag) (err error) {
	// TODO(dfc) named return value is a time bomb
	// TODO(axw) move this logic.
	defer func() {
		if params.IsCodeNotFoundOrCodeUnauthorized(err) {
			err = worker.ErrTerminateAgent
		}
	}()
	if w.unit, err = w.st.Unit(unitTag); err != nil {
		return err
	}
	w.service, err = w.unit.Service()
	if err != nil {
		return err
	}
	return nil
}

func (w *RemoteStateWatcher) loop(unitTag names.UnitTag) (err error) {
	var requiredEvents int

	var seenUnitChange bool
	unitw, err := w.unit.Watch()
	if err != nil {
		return err
	}
	defer watcher.Stop(unitw, &w.tomb)
	requiredEvents++

	var seenServiceChange bool
	servicew, err := w.service.Watch()
	if err != nil {
		return err
	}
	defer watcher.Stop(servicew, &w.tomb)
	requiredEvents++

	var seenConfigChange bool
	configw, err := w.unit.WatchConfigSettings()
	if err != nil {
		return err
	}
	defer watcher.Stop(configw, &w.tomb)
	requiredEvents++

	var seenRelationsChange bool
	relationsw, err := w.service.WatchRelations()
	if err != nil {
		return err
	}
	defer watcher.Stop(relationsw, &w.tomb)
	requiredEvents++

	var seenAddressesChange bool
	addressesw, err := w.unit.WatchAddresses()
	if err != nil {
		return err
	}
	defer watcher.Stop(addressesw, &w.tomb)
	requiredEvents++

	var seenStorageChange bool
	storagew, err := w.unit.WatchStorage()
	if err != nil {
		return err
	}
	defer watcher.Stop(storagew, &w.tomb)
	requiredEvents++

	var seenLeaderSettingsChange bool
	leaderSettingsw, err := w.service.WatchLeadershipSettings()
	if err != nil {
		return err
	}
	defer watcher.Stop(leaderSettingsw, &w.tomb)
	requiredEvents++

	var seenActionsChange bool
	actionsw, err := w.unit.WatchActionNotifications()
	if err != nil {
		return err
	}
	defer watcher.Stop(actionsw, &w.tomb)
	requiredEvents++

	var seenLeadershipChange bool
	// There's no watcher for this per se; we wait on a channel
	// returned by the leadership tracker.
	requiredEvents++

	var eventsObserved int
	observedEvent := func(flag *bool) {
		if !*flag {
			*flag = true
			eventsObserved++
		}
	}

	// fire will, once the first event for each watcher has
	// been observed, send a signal on the out channel.
	fire := func() {
		if eventsObserved != requiredEvents {
			return
		}
		select {
		case w.out <- struct{}{}:
		default:
		}
	}

	defer func() {
		for _, ruw := range w.relations {
			watcher.Stop(ruw, &w.tomb)
		}
	}()

	// Check the initial leadership status, and then we can flip-flop
	// waiting on leader or minion to trigger the changed event.
	var waitLeader, waitMinion <-chan struct{}
	claimLeader := w.leadershipTracker.ClaimLeader()
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case <-claimLeader.Ready():
		isLeader := claimLeader.Wait()
		w.leadershipChanged(isLeader)
		if isLeader {
			waitMinion = w.leadershipTracker.WaitMinion().Ready()
		} else {
			waitLeader = w.leadershipTracker.WaitLeader().Ready()
		}
		observedEvent(&seenLeadershipChange)
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case _, ok := <-unitw.Changes():
			logger.Debugf("got unit change")
			if !ok {
				return watcher.EnsureErr(unitw)
			}
			if err := w.unitChanged(); err != nil {
				return err
			}
			observedEvent(&seenUnitChange)

		case _, ok := <-servicew.Changes():
			logger.Debugf("got service change")
			if !ok {
				return watcher.EnsureErr(servicew)
			}
			if err := w.serviceChanged(); err != nil {
				return err
			}
			observedEvent(&seenServiceChange)

		case _, ok := <-configw.Changes():
			logger.Debugf("got config change: ok=%t", ok)
			if !ok {
				return watcher.EnsureErr(configw)
			}
			if err := w.configChanged(); err != nil {
				return err
			}
			observedEvent(&seenConfigChange)

		case _, ok := <-addressesw.Changes():
			logger.Debugf("got address change: ok=%t", ok)
			if !ok {
				return watcher.EnsureErr(addressesw)
			}
			if err := w.addressesChanged(); err != nil {
				return err
			}
			observedEvent(&seenAddressesChange)

		case _, ok := <-leaderSettingsw.Changes():
			logger.Debugf("got leader settings change: ok=%t", ok)
			if !ok {
				return watcher.EnsureErr(leaderSettingsw)
			}
			if err := w.leaderSettingsChanged(); err != nil {
				return err
			}
			observedEvent(&seenLeaderSettingsChange)

		case actions, ok := <-actionsw.Changes():
			logger.Debugf("got action change: %v ok=%t", actions, ok)
			if !ok {
				return watcher.EnsureErr(actionsw)
			}
			if err := w.actionsChanged(actions); err != nil {
				return err
			}
			observedEvent(&seenActionsChange)

		case keys, ok := <-relationsw.Changes():
			logger.Debugf("got relations change: ok=%t", ok)
			if !ok {
				return watcher.EnsureErr(relationsw)
			}
			if err := w.relationsChanged(keys); err != nil {
				return err
			}
			observedEvent(&seenRelationsChange)

		case keys, ok := <-storagew.Changes():
			logger.Debugf("got storage change: %v ok=%t", keys, ok)
			if !ok {
				return watcher.EnsureErr(storagew)
			}
			if err := w.storageChanged(keys); err != nil {
				return err
			}
			observedEvent(&seenStorageChange)

		case <-waitMinion:
			logger.Debugf("got leadership change: minion")
			if err := w.leadershipChanged(false); err != nil {
				return err
			}
			waitMinion = nil
			waitLeader = w.leadershipTracker.WaitLeader().Ready()

		case <-waitLeader:
			logger.Debugf("got leadership change: leader")
			if err := w.leadershipChanged(true); err != nil {
				return err
			}
			waitLeader = nil
			waitMinion = w.leadershipTracker.WaitMinion().Ready()

		case change := <-w.storageAttachmentChanges:
			logger.Debugf("storage attachment change %v", change)
			if err := w.storageAttachmentChanged(change); err != nil {
				return err
			}

		case change := <-w.relationUnitsChanges:
			logger.Debugf("got a relation units change: %v", change)
			if err := w.relationUnitsChanged(change); err != nil {
				return err
			}

		case <-w.updateStatusChannel():
			logger.Debugf("update status timer triggered")
			if err := w.updateStatusChanged(); err != nil {
				return err
			}
		}

		// Something changed.
		fire()
	}
}

// updateStatusChanged is called when the update status timer expires.
func (w *RemoteStateWatcher) updateStatusChanged() error {
	w.mu.Lock()
	w.current.UpdateStatusRequired = true
	w.mu.Unlock()
	return nil
}

// unitChanged responds to changes in the unit.
func (w *RemoteStateWatcher) unitChanged() error {
	if err := w.unit.Refresh(); err != nil {
		return err
	}
	resolved, err := w.unit.Resolved()
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current.Life = w.unit.Life()
	w.current.ResolvedMode = resolved
	return nil
}

// serviceChanged responds to changes in the service.
func (w *RemoteStateWatcher) serviceChanged() error {
	if err := w.service.Refresh(); err != nil {
		return err
	}
	url, force, err := w.service.CharmURL()
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.current.CharmURL = url
	w.current.ForceCharmUpgrade = force
	w.mu.Unlock()
	return nil
}

func (w *RemoteStateWatcher) configChanged() error {
	w.mu.Lock()
	w.current.ConfigVersion++
	w.mu.Unlock()
	return nil
}

func (w *RemoteStateWatcher) addressesChanged() error {
	w.mu.Lock()
	w.current.ConfigVersion++
	w.mu.Unlock()
	return nil
}

func (w *RemoteStateWatcher) leaderSettingsChanged() error {
	w.mu.Lock()
	w.current.LeaderSettingsVersion++
	w.mu.Unlock()
	return nil
}

func (w *RemoteStateWatcher) leadershipChanged(isLeader bool) error {
	w.mu.Lock()
	w.current.Leader = isLeader
	w.mu.Unlock()
	return nil
}

// relationsChanged responds to service relation changes.
func (w *RemoteStateWatcher) relationsChanged(keys []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, key := range keys {
		relationTag := names.NewRelationTag(key)
		rel, err := w.st.Relation(relationTag)
		if params.IsCodeNotFoundOrCodeUnauthorized(err) {
			// If it's actually gone, this unit cannot have entered
			// scope, and therefore never needs to know about it.
			if ruw, ok := w.relations[relationTag]; ok {
				if err := ruw.Stop(); err != nil {
					return errors.Trace(err)
				}
				delete(w.relations, relationTag)
				delete(w.current.Relations, ruw.relationId)
			}
		} else if err != nil {
			return err
		} else {
			if _, ok := w.relations[relationTag]; ok {
				relationSnapshot := w.current.Relations[rel.Id()]
				relationSnapshot.Life = rel.Life()
				w.current.Relations[rel.Id()] = relationSnapshot
				continue
			}
			in, err := w.st.WatchRelationUnits(relationTag, w.unit.Tag())
			if err != nil {
				return errors.Trace(err)
			}
			if err := w.watchRelationUnits(rel, relationTag, in); err != nil {
				watcher.Stop(in, &w.tomb)
				return errors.Trace(err)
			}
		}
	}
	return nil
}

// watchRelationUnits starts watching the relation units for the given
// relation, waits for its first event, and records the information in
// the current snapshot.
func (w *RemoteStateWatcher) watchRelationUnits(
	rel Relation, relationTag names.RelationTag, in apiwatcher.RelationUnitsWatcher,
) error {
	relationSnapshot := RelationSnapshot{
		Life:    rel.Life(),
		Members: make(map[string]int64),
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case change, ok := <-in.Changes():
		if !ok {
			return watcher.EnsureErr(in)
		}
		for unit, settings := range change.Changed {
			relationSnapshot.Members[unit] = settings.Version
		}
	}
	w.current.Relations[rel.Id()] = relationSnapshot
	w.relations[relationTag] = newRelationUnitsWatcher(
		rel.Id(), in, w.relationUnitsChanges,
	)
	return nil
}

// relationUnitsChanged responds to relation units changes.
func (w *RemoteStateWatcher) relationUnitsChanged(change relationUnitsChange) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	snapshot, ok := w.current.Relations[change.relationId]
	if !ok {
		return nil
	}
	for unit, settings := range change.Changed {
		snapshot.Members[unit] = settings.Version
	}
	for _, unit := range change.Departed {
		delete(snapshot.Members, unit)
	}
	return nil
}

// storageAttachmentChanged responds to storage attachment changes.
func (w *RemoteStateWatcher) storageAttachmentChanged(change storageAttachmentChange) error {
	w.mu.Lock()
	w.current.Storage[change.Tag] = change.Snapshot
	w.mu.Unlock()
	return nil
}

func (w *RemoteStateWatcher) actionsChanged(actions []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current.Actions = append(w.current.Actions, actions...)
	return nil
}

// storageChanged responds to unit storage changes.
func (w *RemoteStateWatcher) storageChanged(keys []string) error {
	tags := make([]names.StorageTag, len(keys))
	for i, key := range keys {
		tags[i] = names.NewStorageTag(key)
	}
	ids := make([]params.StorageAttachmentId, len(keys))
	for i, tag := range tags {
		ids[i] = params.StorageAttachmentId{
			StorageTag: tag.String(),
			UnitTag:    w.unit.Tag().String(),
		}
	}
	results, err := w.st.StorageAttachmentLife(ids)
	if err != nil {
		return errors.Trace(err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for i, result := range results {
		tag := tags[i]
		if result.Error == nil {
			if storageSnapshot, ok := w.current.Storage[tag]; ok {
				// We've previously started a watcher for this storage
				// attachment, so all we needed to do was update the
				// lifecycle state.
				storageSnapshot.Life = result.Life
				w.current.Storage[tag] = storageSnapshot
				continue
			}
			// We haven't seen this storage attachment before, so start
			// a watcher now and wait for the initial event.
			in, err := w.st.WatchStorageAttachment(tag, w.unit.Tag())
			if err != nil {
				return errors.Annotate(err, "watching storage attachment")
			}
			if err := w.watchStorageAttachment(tag, result.Life, in); err != nil {
				watcher.Stop(in, &w.tomb)
				return errors.Trace(err)
			}
		} else if params.IsCodeNotFound(result.Error) {
			if watcher, ok := w.storageAttachmentWatchers[tag]; ok {
				if err := watcher.Stop(); err != nil {
					return errors.Annotatef(
						err, "stopping watcher of %s attachment",
						names.ReadableString(tag),
					)
				}
				delete(w.storageAttachmentWatchers, tag)
			}
			delete(w.current.Storage, tag)
		} else {
			return errors.Annotatef(
				result.Error, "getting life of %s attachment",
				names.ReadableString(tag),
			)
		}
	}
	return nil
}

// watchStorageAttachment starts watching the storage attachment with
// the specified storage tag, waits for its first event, and records
// the information in the current snapshot.
func (w *RemoteStateWatcher) watchStorageAttachment(
	tag names.StorageTag,
	life params.Life,
	in apiwatcher.NotifyWatcher,
) error {
	var storageSnapshot StorageSnapshot
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case _, ok := <-in.Changes():
		if !ok {
			return watcher.EnsureErr(in)
		}
		var err error
		storageSnapshot, err = getStorageSnapshot(w.st, tag, w.unit.Tag())
		if params.IsCodeNotProvisioned(err) {
			// If the storage is unprovisioned, we still want to
			// record the attachment, but we'll mark it as
			// unattached. This allows the uniter to wait for
			// pending storage attachments to be provisioned.
			storageSnapshot = StorageSnapshot{Life: life}
		} else if err != nil {
			return errors.Annotatef(err, "processing initial storage attachment change")
		}
	}
	w.current.Storage[tag] = storageSnapshot
	w.storageAttachmentWatchers[tag] = newStorageAttachmentWatcher(
		w.st, in, w.unit.Tag(), tag, w.storageAttachmentChanges,
	)
	return nil
}
