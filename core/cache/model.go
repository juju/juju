// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sort"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
)

const modelConfigChange = "model-config-change"

func newModel(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Model {
	m := &Model{
		metrics: metrics,
		// TODO: consider a separate hub per model for better scalability
		// when many models.
		hub:          hub,
		applications: make(map[string]*Application),
		units:        make(map[string]*Unit),
	}
	return m
}

// Model is a cached model in the controller. The model is kept up to
// date with changes flowing into the cached controller.
type Model struct {
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details      ModelChange
	configHash   string
	hashCache    *modelConfigHashCache
	applications map[string]*Application
	units        map[string]*Unit
}

// Report returns information that is used in the dependency engine report.
func (m *Model) Report() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	return map[string]interface{}{
		"name":              m.details.Owner + "/" + m.details.Name,
		"life":              m.details.Life,
		"application-count": len(m.applications),
		"unit-count":        len(m.units),
	}
}

// Application returns the application for the input name.
// If the application is not found, a NotFoundError is returned.
func (m *Model) Application(appName string) (*Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	app, found := m.applications[appName]
	if !found {
		return nil, errors.NotFoundf("application %q", appName)
	}
	return app, nil
}

// Unit returns the unit with the input name.
// If the unit is not found, a NotFoundError is returned.
func (m *Model) Unit(unitName string) (*Unit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	unit, found := m.units[unitName]
	if !found {
		return nil, errors.NotFoundf("unit %q", unitName)
	}
	return unit, nil
}

// updateApplication adds or updates the application in the model.
func (m *Model) updateApplication(ch ApplicationChange) {
	m.mu.Lock()

	app, found := m.applications[ch.Name]
	if !found {
		app = newApplication(m.metrics, m.hub)
		m.applications[ch.Name] = app
	}
	app.setDetails(ch)

	m.mu.Unlock()
}

// removeApplication removes the application from the model.
func (m *Model) removeApplication(ch RemoveApplication) {
	m.mu.Lock()
	delete(m.applications, ch.Name)
	m.mu.Unlock()
}

// updateUnit adds or updates the unit in the model.
func (m *Model) updateUnit(ch UnitChange) {
	m.mu.Lock()

	unit, found := m.units[ch.Name]
	if !found {
		unit = newUnit(m.metrics, m.hub)
		m.units[ch.Name] = unit
	}
	unit.setDetails(ch)

	m.mu.Unlock()
}

// removeUnit removes the unit from the model.
func (m *Model) removeUnit(ch RemoveUnit) {
	m.mu.Lock()
	delete(m.units, ch.Name)
	m.mu.Unlock()
}

// modelTopic prefixes the topic with the model UUID.
func (m *Model) modelTopic(topic string) string {
	return m.details.ModelUUID + ":" + topic
}

func (m *Model) setDetails(details ModelChange) {
	m.mu.Lock()

	m.details = details
	hashCache, configHash := newModelConfigHashCache(m.metrics, details.Config)
	if configHash != m.configHash {
		m.configHash = configHash
		m.hashCache = hashCache
		m.hub.Publish(m.modelTopic(modelConfigChange), hashCache)
	}

	m.mu.Unlock()
}

// Config returns the current model config.
func (m *Model) Config() map[string]interface{} {
	m.mu.Lock()
	m.metrics.ModelConfigReads.Inc()
	m.mu.Unlock()
	return m.details.Config
}

// WatchConfig creates a watcher for the model config.
// If keys are specified, the watcher is only signals a change when
// those keys change values. If no keys are specified, any change in the
// config will trigger the watcher.
func (m *Model) WatchConfig(keys ...string) *modelConfigWatcher {
	w := newModelConfigWatcher(keys, m.hashCache.getHash(keys))

	unsub := m.hub.Subscribe(m.modelTopic(modelConfigChange), w.configChanged)

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		return nil
	})

	return w
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
	*notifyWatcherBase

	keys []string
	hash string
}

func newModelConfigWatcher(keys []string, keyHash string) *modelConfigWatcher {
	sort.Strings(keys)

	return &modelConfigWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),

		keys: keys,
		hash: keyHash,
	}
}

func (w *modelConfigWatcher) configChanged(topic string, value interface{}) {
	hashCache, ok := value.(*modelConfigHashCache)
	if !ok {
		logger.Errorf("programming error, value not of type *modelConfigHashCache")
	}
	hash := hashCache.getHash(w.keys)
	if hash == w.hash {
		// Nothing that we care about has changed, so we're done.
		return
	}
	w.notify()
}
