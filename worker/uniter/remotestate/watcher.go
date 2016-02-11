// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
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
	commandChannel            <-chan string
	retryHookChannel          <-chan struct{}

	catacomb catacomb.Catacomb

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
	CommandChannel      <-chan string
	RetryHookChannel    <-chan struct{}
	UnitTag             names.UnitTag
}

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
		commandChannel:            config.CommandChannel,
		retryHookChannel:          config.RetryHookChannel,
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
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			return w.loop(config.UnitTag)
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *RemoteStateWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *RemoteStateWatcher) Wait() error {
	return w.catacomb.Wait()
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
	copy(snapshot.Actions, w.current.Actions)
	snapshot.Commands = make([]string, len(w.current.Commands))
	copy(snapshot.Commands, w.current.Commands)
	return snapshot
}

func (w *RemoteStateWatcher) ClearResolvedMode() {
	w.mu.Lock()
	w.current.ResolvedMode = params.ResolvedNone
	w.mu.Unlock()
}

func (w *RemoteStateWatcher) CommandCompleted(completed string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, id := range w.current.Commands {
		if id != completed {
			continue
		}
		w.current.Commands = append(
			w.current.Commands[:i],
			w.current.Commands[i+1:]...,
		)
		break
	}
}

func (w *RemoteStateWatcher) setUp(unitTag names.UnitTag) (err error) {
	// TODO(dfc) named return value is a time bomb
	// TODO(axw) move this logic.
	defer func() {
		cause := errors.Cause(err)
		if params.IsCodeNotFoundOrCodeUnauthorized(cause) {
			err = worker.ErrTerminateAgent
		}
	}()
	if w.unit, err = w.st.Unit(unitTag); err != nil {
		return errors.Trace(err)
	}
	w.service, err = w.unit.Service()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (w *RemoteStateWatcher) loop(unitTag names.UnitTag) (err error) {
	if err := w.setUp(unitTag); err != nil {
		return errors.Trace(err)
	}

	var requiredEvents int

	var seenUnitChange bool
	unitw, err := w.unit.Watch()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(unitw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenServiceChange bool
	servicew, err := w.service.Watch()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(servicew); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenConfigChange bool
	configw, err := w.unit.WatchConfigSettings()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(configw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenRelationsChange bool
	relationsw, err := w.service.WatchRelations()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(relationsw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenAddressesChange bool
	addressesw, err := w.unit.WatchAddresses()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(addressesw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenStorageChange bool
	storagew, err := w.unit.WatchStorage()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(storagew); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenLeaderSettingsChange bool
	leaderSettingsw, err := w.service.WatchLeadershipSettings()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(leaderSettingsw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenActionsChange bool
	actionsw, err := w.unit.WatchActionNotifications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(actionsw); err != nil {
		return errors.Trace(err)
	}
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

	// Check the initial leadership status, and then we can flip-flop
	// waiting on leader or minion to trigger the changed event.
	var waitLeader, waitMinion <-chan struct{}
	claimLeader := w.leadershipTracker.ClaimLeader()
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
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
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-unitw.Changes():
			logger.Debugf("got unit change")
			if !ok {
				return errors.New("unit watcher closed")
			}
			if err := w.unitChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenUnitChange)

		case _, ok := <-servicew.Changes():
			logger.Debugf("got service change")
			if !ok {
				return errors.New("service watcher closed")
			}
			if err := w.serviceChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenServiceChange)

		case _, ok := <-configw.Changes():
			logger.Debugf("got config change: ok=%t", ok)
			if !ok {
				return errors.New("config watcher closed")
			}
			if err := w.configChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenConfigChange)

		case _, ok := <-addressesw.Changes():
			logger.Debugf("got address change: ok=%t", ok)
			if !ok {
				return errors.New("addresses watcher closed")
			}
			if err := w.addressesChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenAddressesChange)

		case _, ok := <-leaderSettingsw.Changes():
			logger.Debugf("got leader settings change: ok=%t", ok)
			if !ok {
				return errors.New("leader settings watcher closed")
			}
			if err := w.leaderSettingsChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenLeaderSettingsChange)

		case actions, ok := <-actionsw.Changes():
			logger.Debugf("got action change: %v ok=%t", actions, ok)
			if !ok {
				return errors.New("actions watcher closed")
			}
			if err := w.actionsChanged(actions); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenActionsChange)

		case keys, ok := <-relationsw.Changes():
			logger.Debugf("got relations change: ok=%t", ok)
			if !ok {
				return errors.New("relations watcher closed")
			}
			if err := w.relationsChanged(keys); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenRelationsChange)

		case keys, ok := <-storagew.Changes():
			logger.Debugf("got storage change: %v ok=%t", keys, ok)
			if !ok {
				return errors.New("storage watcher closed")
			}
			if err := w.storageChanged(keys); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenStorageChange)

		case <-waitMinion:
			logger.Debugf("got leadership change: minion")
			if err := w.leadershipChanged(false); err != nil {
				return errors.Trace(err)
			}
			waitMinion = nil
			waitLeader = w.leadershipTracker.WaitLeader().Ready()

		case <-waitLeader:
			logger.Debugf("got leadership change: leader")
			if err := w.leadershipChanged(true); err != nil {
				return errors.Trace(err)
			}
			waitLeader = nil
			waitMinion = w.leadershipTracker.WaitMinion().Ready()

		case change := <-w.storageAttachmentChanges:
			logger.Debugf("storage attachment change %v", change)
			if err := w.storageAttachmentChanged(change); err != nil {
				return errors.Trace(err)
			}

		case change := <-w.relationUnitsChanges:
			logger.Debugf("got a relation units change: %v", change)
			if err := w.relationUnitsChanged(change); err != nil {
				return errors.Trace(err)
			}

		case <-w.updateStatusChannel():
			logger.Debugf("update status timer triggered")
			if err := w.updateStatusChanged(); err != nil {
				return errors.Trace(err)
			}

		case id := <-w.commandChannel:
			logger.Debugf("command enqueued: %v", id)
			if err := w.commandsChanged(id); err != nil {
				return err
			}

		case <-w.retryHookChannel:
			logger.Debugf("retry hook timer triggered")
			if err := w.retryHookTimerTriggered(); err != nil {
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
	w.current.UpdateStatusVersion++
	w.mu.Unlock()
	return nil
}

// commandsChanged is called when a command is enqueued.
func (w *RemoteStateWatcher) commandsChanged(id string) error {
	w.mu.Lock()
	w.current.Commands = append(w.current.Commands, id)
	w.mu.Unlock()
	return nil
}

// retryHookTimerTriggered is called when the retry hook timer expires.
func (w *RemoteStateWatcher) retryHookTimerTriggered() error {
	w.mu.Lock()
	w.current.RetryHookVersion++
	w.mu.Unlock()
	return nil
}

// unitChanged responds to changes in the unit.
func (w *RemoteStateWatcher) unitChanged() error {
	if err := w.unit.Refresh(); err != nil {
		return errors.Trace(err)
	}
	resolved, err := w.unit.Resolved()
	if err != nil {
		return errors.Trace(err)
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
		return errors.Trace(err)
	}
	url, force, err := w.service.CharmURL()
	if err != nil {
		return errors.Trace(err)
	}
	ver, err := w.service.CharmModifiedVersion()
	if err != nil {
		return errors.Trace(err)
	}
	w.mu.Lock()
	w.current.CharmURL = url
	w.current.ForceCharmUpgrade = force
	w.current.CharmModifiedVersion = ver
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
				worker.Stop(ruw)
				delete(w.relations, relationTag)
				delete(w.current.Relations, ruw.relationId)
			}
		} else if err != nil {
			return errors.Trace(err)
		} else {
			if _, ok := w.relations[relationTag]; ok {
				relationSnapshot := w.current.Relations[rel.Id()]
				relationSnapshot.Life = rel.Life()
				w.current.Relations[rel.Id()] = relationSnapshot
				continue
			}
			ruw, err := w.st.WatchRelationUnits(relationTag, w.unit.Tag())
			if err != nil {
				return errors.Trace(err)
			}
			// Because of the delay before handing off responsibility to
			// newRelationUnitsWatcher below, add to our own catacomb to
			// ensure errors get picked up if they happen.
			if err := w.catacomb.Add(ruw); err != nil {
				return errors.Trace(err)
			}
			if err := w.watchRelationUnits(rel, relationTag, ruw); err != nil {
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
	rel Relation, relationTag names.RelationTag, ruw watcher.RelationUnitsWatcher,
) error {
	relationSnapshot := RelationSnapshot{
		Life:    rel.Life(),
		Members: make(map[string]int64),
	}
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case change, ok := <-ruw.Changes():
		if !ok {
			return errors.New("relation units watcher closed")
		}
		for unit, settings := range change.Changed {
			relationSnapshot.Members[unit] = settings.Version
		}
	}
	innerRUW, err := newRelationUnitsWatcher(rel.Id(), ruw, w.relationUnitsChanges)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(innerRUW); err != nil {
		return errors.Trace(err)
	}
	w.current.Relations[rel.Id()] = relationSnapshot
	w.relations[relationTag] = innerRUW
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
			// a watcher now; add it to our catacomb in case of mishap;
			// and wait for the initial event.
			saw, err := w.st.WatchStorageAttachment(tag, w.unit.Tag())
			if err != nil {
				return errors.Annotate(err, "watching storage attachment")
			}
			if err := w.catacomb.Add(saw); err != nil {
				return errors.Trace(err)
			}
			if err := w.watchStorageAttachment(tag, result.Life, saw); err != nil {
				return errors.Trace(err)
			}
		} else if params.IsCodeNotFound(result.Error) {
			if watcher, ok := w.storageAttachmentWatchers[tag]; ok {
				// already under catacomb management, any error tracked already
				worker.Stop(watcher)
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
	saw watcher.NotifyWatcher,
) error {
	var storageSnapshot StorageSnapshot
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case _, ok := <-saw.Changes():
		if !ok {
			return errors.New("storage attachment watcher closed")
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
	innerSAW, err := newStorageAttachmentWatcher(
		w.st, saw, w.unit.Tag(), tag, w.storageAttachmentChanges,
	)
	if err != nil {
		return errors.Trace(err)
	}
	w.current.Storage[tag] = storageSnapshot
	w.storageAttachmentWatchers[tag] = innerSAW
	return nil
}
