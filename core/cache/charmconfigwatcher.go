// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"

	"github.com/juju/juju/core/settings"
)

type charmConfigModel interface {
	Application(string) (Application, error)
	Branches() []Branch
	Metrics() *ControllerGauges
}

// charmConfigWatchConfig contains data required for a
// CharmConfigWatcher to operate.
type charmConfigWatcherConfig struct {
	model charmConfigModel

	unitName string
	appName  string
	charmURL string

	// appConfigChangeTopic is the pub/sub topic to which the watcher will
	// listen for application charm config change messages.
	appConfigChangeTopic string
	// branchChangeTopic is the pub/sub topic to which the watcher will
	// listen for model branch change messages.
	branchChangeTopic string
	// branchRemoveTopic is the pub/sub topic to which the watcher will
	// listen for model branch removal messages.
	branchRemoveTopic string

	// hub is the pub/sub hub on which the watcher will receive messages
	// before determining whether to notify.
	hub *pubsub.SimpleHub
	// res is the cache resident responsible for creating this watcher.
	res *Resident
}

// CharmConfigWatcher watches application charm config on behalf of a unit.
// The watcher will notify if either of the following events cause a change
// to the unit's effective configuration:
// - Changes to the charm config settings for the unit's application.
// - Changes to a model branch being tracked by the unit.
type CharmConfigWatcher struct {
	*stringsWatcherBase

	// initComplete is a channel that will be closed when the
	// watcher is fully constructed and ready to handle events.
	initComplete chan struct{}

	unitName   string
	appName    string
	charmURL   string
	branchName string

	CharmConfigHashCacheHitInc  func()
	CharmConfigHashCacheMissInc func()

	masterSettings map[string]interface{}
	branchDeltas   settings.ItemChanges
	configHash     string
}

// newUnitConfigWatcher returns a new watcher for the unit indicated in the
// input configuration.
func newCharmConfigWatcher(cfg charmConfigWatcherConfig) (*CharmConfigWatcher, error) {
	w := &CharmConfigWatcher{
		stringsWatcherBase: &stringsWatcherBase{changes: make(chan []string, 1)},
		initComplete:       make(chan struct{}),
		unitName:           cfg.unitName,
		appName:            cfg.appName,
		charmURL:           cfg.charmURL,
	}

	deregister := cfg.res.registerWorker(w)

	multi := cfg.hub.NewMultiplexer()
	multi.Add(cfg.appConfigChangeTopic, w.appConfigChanged)
	multi.Add(cfg.branchChangeTopic, w.branchChanged)
	multi.Add(cfg.branchRemoveTopic, w.branchRemoved)

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		multi.Unsubscribe()
		deregister()
		return nil
	})

	if err := w.init(cfg.model); err != nil {
		_ = w.Stop()
		return nil, errors.Trace(err)
	}
	return w, nil
}

// init determines baseline master configuration, branch tracking settings
// and the configuration hash for the watcher's unit.
// It then closes the init channel to indicate the watcher is operational.
func (w *CharmConfigWatcher) init(model charmConfigModel) error {
	app, err := model.Application(w.appName)
	if err != nil {
		return errors.Trace(err)
	}
	w.masterSettings = app.Config()

	w.CharmConfigHashCacheHitInc = model.Metrics().CharmConfigHashCacheHit.Inc
	w.CharmConfigHashCacheMissInc = model.Metrics().CharmConfigHashCacheMiss.Inc

	branches := model.Branches()
	for _, b := range branches {
		if w.isTracking(b) {
			w.branchName = b.Name()
			w.branchDeltas = b.AppConfig(w.appName)
			break
		}
	}

	// Always notify with the first hash.
	if _, err := w.setConfigHash(); err != nil {
		return errors.Trace(err)
	}
	w.notify([]string{w.configHash})

	close(w.initComplete)
	return nil
}

// appConfigChanged is called when a message is received indicating changed
// application master charm configuration.
func (w *CharmConfigWatcher) appConfigChanged(_ string, msg interface{}) {
	if !w.waitInitOrDying() {
		return
	}

	hashCache, ok := msg.(*hashCache)
	if !ok {
		logger.Errorf("programming error; application config message was not of expected type, *hashCache")
		return
	}

	w.masterSettings = hashCache.config
	w.checkConfig()
}

// branchChanged is called when we receive a message to say that a branch has
// been updated in the cache.
// If it is the branch that this watcher's unit is tracking,
// check if the latest config delta warrants a notification.
func (w *CharmConfigWatcher) branchChanged(_ string, msg interface{}) {
	if !w.waitInitOrDying() {
		return
	}

	b, okUnit := msg.(Branch)
	if !okUnit {
		logger.Errorf("programming error; branch change message was not of expected type, Branch")
		return
	}

	// If we do not know whether we are tracking this branch, find out.
	if w.branchName == "" && w.isTracking(b) {
		w.branchName = b.Name()
	}
	if w.branchName != b.Name() {
		return
	}

	w.branchDeltas = b.AppConfig(w.appName)
	w.checkConfig()
}

// branchRemoved is called when we receive a message to say that a branch has
// been removed from the cache.
// If this watcher's unit was tracking the branch, clean the branch-based
// details and check if the resulting settings warrant a notification.
func (w *CharmConfigWatcher) branchRemoved(topic string, msg interface{}) {
	if !w.waitInitOrDying() {
		return
	}

	name, okUnit := msg.(string)
	if !okUnit {
		logger.Errorf("programming error; branch deleted message was not of expected type, string")
		return
	}

	if w.branchName != name {
		return
	}

	// The branch we are tracking was deleted.
	// Since we know that a branch with tracking units can not be aborted,
	// the branch must have been committed.
	// This means that we can anticipate a message for a master settings change
	// (it may even have preceded this event), so just clear the branch info
	// without reevaluating the hash.
	w.branchName = ""
	w.branchDeltas = nil
}

// isTracking returns true if this watcher's unit is tracking the input branch.
func (w *CharmConfigWatcher) isTracking(b Branch) bool {
	units := b.AssignedUnits()[w.appName]
	if len(units) == 0 {
		return false
	}
	return set.NewStrings(units...).Contains(w.unitName)
}

// checkConfig generates a new hash based on current effective configuration.
// If the hash has changed, a notification is sent.
func (w *CharmConfigWatcher) checkConfig() {
	changed, err := w.setConfigHash()
	if err != nil {
		logger.Errorf("generating hash for charm config: %s", errors.ErrorStack(err))
		return
	}
	if changed {
		w.notify([]string{w.configHash})
	}
}

// setConfigHash applies any known branch deltas to the master charm config,
// Then compares a hash of the result with the last known config hash.
// The boolean return indicates whether the hash has changed.
func (w *CharmConfigWatcher) setConfigHash() (bool, error) {
	cfg := copyDataMap(w.masterSettings)
	for _, delta := range w.branchDeltas {
		switch {
		case delta.IsAddition(), delta.IsModification():
			cfg[delta.Key] = delta.NewValue
		case delta.IsDeletion():
			delete(cfg, delta.Key)
		}
	}

	newHash, err := hashSettings(cfg, w.charmURL)
	if err != nil {
		return false, errors.Trace(err)
	}
	if w.configHash == newHash {
		w.CharmConfigHashCacheHitInc()
		return false, nil
	}
	w.CharmConfigHashCacheMissInc()
	w.configHash = newHash
	return true, nil
}

// waitInitOrDying returns true when the watcher is fully initialised,
// or false if it is dying.
func (w *CharmConfigWatcher) waitInitOrDying() bool {
	select {
	case <-w.initComplete:
		return true
	case <-w.tomb.Dying():
		return false
	}
}
