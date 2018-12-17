// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sort"
	"strings"
	"sync"

	"github.com/juju/pubsub"
	"gopkg.in/tomb.v2"
)

const modelConfigChange = "model-config-change"

func newModel(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Model {
	m := &Model{
		metrics: metrics,
		hub:     hub,
	}
	return m
}

// Model is a cached model in the controller. The model is kept up to
// date with changes flowing into the cached controller.
type Model struct {
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details    ModelChange
	configHash string
	hashCache  *modelConfigHashCache
}

// Report returns information that is used in the dependency engine report.
func (m *Model) Report() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]interface{}{
		"name": m.details.Owner + "/" + m.details.Name,
		"life": m.details.Life,
	}
}

// modelTopic prefixes the topic with the model UUID.
func (m *Model) modelTopic(topic string) string {
	return m.details.ModelUUID + ":" + topic
}

func (m *Model) setDetails(details ModelChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.details = details

	hashCache, configHash := newModelConfigHashCache(m.metrics, details.Config)
	if configHash != m.configHash {
		m.configHash = configHash
		m.hashCache = hashCache
		m.hub.Publish(m.modelTopic(modelConfigChange), hashCache)
	}
}

// Config returns the current model config.
func (m *Model) Config() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics.ModelConfigReads.Inc()
	return m.details.Config
}

// WatchConfig creates a watcher for the model config.
// If keys are specified, the watcher is only signals a change when
// those keys change values. If no keys are specified, any change in the
// config will trigger the watcher.
func (m *Model) WatchConfig(keys ...string) *modelConfigWatcher {
	// We use a single entry buffered channel for the changes.
	// This allows the config changed handler to send a value when there
	// is a change, but if that value hasn't been consumed before the
	// next change, the second change is discarded.
	sort.Strings(keys)
	watcher := &modelConfigWatcher{
		keys:    keys,
		changes: make(chan struct{}, 1),
	}
	watcher.hash = m.hashCache.getHash(keys)
	// Send initial event down the channel. We know that this will
	// execute immediately because it is a buffered channel.
	watcher.changes <- struct{}{}

	unsub := m.hub.Subscribe(m.modelTopic(modelConfigChange), watcher.configChanged)

	watcher.tomb.Go(func() error {
		<-watcher.tomb.Dying()
		unsub()
		return nil
	})

	return watcher
}

type modelConfigHashCache struct {
	metrics *ControllerGauges
	config  map[string]interface{}
	// The key to the hash map is the stringified keys of the watcher.
	// They should be sorted and comma delimited.
	hash map[string]string
	mu   sync.Mutex
}

func newModelConfigHashCache(metrics *ControllerGauges, config map[string]interface{}) (*modelConfigHashCache, string) {
	configCache := &modelConfigHashCache{
		metrics: metrics,
		config:  config,
		hash:    make(map[string]string),
	}
	// Generate the hash for the entire config.
	allHash := configCache.generateHash(nil)
	configCache.hash[""] = allHash
	return configCache, allHash
}

func (c *modelConfigHashCache) getHash(keys []string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := strings.Join(keys, ",")
	value, found := c.hash[key]
	if found {
		c.metrics.ModelHashCacheHit.Inc()
		return value
	}
	value = c.generateHash(keys)
	c.hash[key] = value
	return value
}

func (c *modelConfigHashCache) generateHash(keys []string) string {
	// We are generating a hash, so call it a miss.
	c.metrics.ModelHashCacheMiss.Inc()

	interested := c.config
	if len(keys) > 0 {
		interested = make(map[string]interface{})
		for _, key := range keys {
			if value, found := c.config[key]; found {
				interested[key] = value
			}
		}
	}
	h, err := hash(interested)
	if err != nil {
		logger.Errorf("invariant error - model config should be yaml serializable and hashable, %v", err)
		return ""
	}
	return h
}

type modelConfigWatcher struct {
	keys    []string
	hash    string
	tomb    tomb.Tomb
	changes chan struct{}
	// We can't send down a closed channel, so protect the sending
	// with a mutex and bool. Since you can't really even ask a channel
	// if it is closed.
	closed bool
	mu     sync.Mutex
}

func (w *modelConfigWatcher) configChanged(topic string, value interface{}) {
	hashCache, ok := value.(*modelConfigHashCache)
	if !ok {
		logger.Errorf("programming error, value not a *modelConfigHashCache")
	}
	hash := hashCache.getHash(w.keys)
	if hash == w.hash {
		// Nothing that we care about has changed, so we're done.
		return
	}
	// Let the listener know.
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}

	select {
	case w.changes <- struct{}{}:
	default:
		// Already a pending change, so do nothing.
	}
}

// Changes is part of the core watcher definition.
// The changes channel is never closed.
func (w *modelConfigWatcher) Changes() <-chan struct{} {
	return w.changes
}

// Kill is part of the worker.Worker interface.
func (w *modelConfigWatcher) Kill() {
	w.mu.Lock()
	w.closed = true
	close(w.changes)
	w.mu.Unlock()
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *modelConfigWatcher) Wait() error {
	return w.tomb.Wait()
}

// Stop is currently required by the Resources wrapper in the apiserver.
func (w *modelConfigWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}
