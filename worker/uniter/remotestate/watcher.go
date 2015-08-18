// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.uniter.remotestate")

// remoteStateWatcher collects unit, service, and service config information
// from separate state watchers, and updates a Snapshot which is sent on a
// channel upon change.
type remoteStateWatcher struct {
	st      *uniter.State
	unit    *uniter.Unit
	service *uniter.Service
	tomb    tomb.Tomb

	out     chan struct{}
	mu      sync.Mutex
	current Snapshot
}

// NewFilter returns a remoteStateWatcher that handles state changes pertaining to the
// supplied unit.
func NewWatcher(st *uniter.State, unitTag names.UnitTag) (Watcher, error) {
	w := &remoteStateWatcher{
		st:  st,
		out: make(chan struct{}),
		current: Snapshot{
			Relations: make(map[int]params.Life),
			Storage:   make(map[names.StorageTag]params.Life),
		},
	}
	if err := w.init(unitTag); err != nil {
		return nil, errors.Trace(err)
	}
	go func() {
		defer w.tomb.Done()
		err := w.loop(unitTag)
		logger.Errorf("remote state watcher exited: %v", err)
		w.tomb.Kill(err)
	}()
	return w, nil
}

func (w *remoteStateWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *remoteStateWatcher) Dead() <-chan struct{} {
	return w.tomb.Dead()
}

func (w *remoteStateWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *remoteStateWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *remoteStateWatcher) RemoteStateChanged() <-chan struct{} {
	return w.out
}

func (w *remoteStateWatcher) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	snapshot := w.current
	snapshot.Relations = make(map[int]params.Life)
	for id, life := range w.current.Relations {
		snapshot.Relations[id] = life
	}
	snapshot.Storage = make(map[names.StorageTag]params.Life)
	for tag, life := range w.current.Storage {
		snapshot.Storage[tag] = life
	}
	return snapshot
}

func (w *remoteStateWatcher) fire() {
	select {
	case w.out <- struct{}{}:
	default:
	}
}

func (w *remoteStateWatcher) init(unitTag names.UnitTag) (err error) {
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
	if err = w.unitChanged(); err != nil {
		return err
	}
	if err = w.serviceChanged(); err != nil {
		return err
	}
	w.fire()
	return nil
}

func (w *remoteStateWatcher) loop(unitTag names.UnitTag) (err error) {
	unitw, err := w.unit.Watch()
	if err != nil {
		return err
	}
	defer watcher.Stop(unitw, &w.tomb)

	servicew, err := w.service.Watch()
	if err != nil {
		return err
	}
	defer watcher.Stop(servicew, &w.tomb)

	configw, err := w.unit.WatchConfigSettings()
	if err != nil {
		return err
	}
	defer watcher.Stop(configw, &w.tomb)

	relationsw, err := w.service.WatchRelations()
	if err != nil {
		return err
	}
	defer watcher.Stop(relationsw, &w.tomb)

	addressesw, err := w.unit.WatchAddresses()
	if err != nil {
		return err
	}
	defer watcher.Stop(addressesw, &w.tomb)

	storagew, err := w.unit.WatchStorage()
	if err != nil {
		return err
	}
	defer watcher.Stop(storagew, &w.tomb)

	leaderSettingsw, err := w.st.LeadershipSettings.WatchLeadershipSettings(w.service.Tag().Id())
	if err != nil {
		return err
	}
	defer watcher.Stop(leaderSettingsw, &w.tomb)

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
		case _, ok := <-servicew.Changes():
			logger.Debugf("got service change")
			if !ok {
				return watcher.EnsureErr(servicew)
			}
			if err := w.serviceChanged(); err != nil {
				return err
			}
		case _, ok := <-configw.Changes():
			logger.Debugf("got config change")
			if !ok {
				return watcher.EnsureErr(configw)
			}
			if err := w.configChanged(); err != nil {
				return err
			}
		case _, ok := <-addressesw.Changes():
			logger.Debugf("got address change")
			if !ok {
				return watcher.EnsureErr(addressesw)
			}
			if err := w.addressesChanged(); err != nil {
				return err
			}
		case _, ok := <-leaderSettingsw.Changes():
			logger.Debugf("got leader settings change: ok=%t", ok)
			if !ok {
				return watcher.EnsureErr(leaderSettingsw)
			}
			if err := w.leaderSettingsChanged(); err != nil {
				return err
			}
		case keys, ok := <-relationsw.Changes():
			logger.Debugf("got relations change")
			if !ok {
				return watcher.EnsureErr(relationsw)
			}
			if err := w.relationsChanged(keys); err != nil {
				return err
			}
		case keys, ok := <-storagew.Changes():
			logger.Debugf("got storage change")
			if !ok {
				return watcher.EnsureErr(storagew)
			}
			if err := w.storageChanged(keys); err != nil {
				return err
			}
		}
		// Something changed.
		w.fire()
	}
}

// unitChanged responds to changes in the unit.
func (w *remoteStateWatcher) unitChanged() error {
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
func (w *remoteStateWatcher) serviceChanged() error {
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

func (w *remoteStateWatcher) configChanged() error {
	w.mu.Lock()
	w.current.ConfigVersion++
	w.mu.Unlock()
	return nil
}

func (w *remoteStateWatcher) addressesChanged() error {
	w.mu.Lock()
	w.current.ConfigVersion++
	w.mu.Unlock()
	return nil
}

func (w *remoteStateWatcher) leaderSettingsChanged() error {
	w.mu.Lock()
	w.current.LeaderSettingsVersion++
	w.mu.Unlock()
	return nil
}

// relationsChanged responds to service relation changes.
func (w *remoteStateWatcher) relationsChanged(keys []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, key := range keys {
		relationTag := names.NewRelationTag(key)
		rel, err := w.st.Relation(relationTag)
		if params.IsCodeNotFoundOrCodeUnauthorized(err) {
			// If it's actually gone, this unit cannot have entered
			// scope, and therefore never needs to know about it.
		} else if err != nil {
			return err
		} else {
			w.current.Relations[rel.Id()] = rel.Life()
		}
	}
	return nil
}

// storageChanged responds to unit storage changes.
func (w *remoteStateWatcher) storageChanged(keys []string) error {
	ids := make([]params.StorageAttachmentId, len(keys))
	for i, key := range keys {
		ids[i] = params.StorageAttachmentId{
			StorageTag: key,
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
		tag := names.NewStorageTag(ids[i].StorageTag)
		if result.Error == nil {
			w.current.Storage[tag] = result.Life
		} else if params.IsCodeNotFound(result.Error) {
			delete(w.current.Storage, tag)
		}
		return errors.Annotatef(
			result.Error, "getting life of %s attachment",
			names.ReadableString(tag),
		)
	}
	return nil
}
