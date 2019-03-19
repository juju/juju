// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
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
		machines:     make(map[string]*Machine),
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
	hashCache    *hashCache
	applications map[string]*Application
	machines     map[string]*Machine
	units        map[string]*Unit
}

// Config returns the current model config.
func (m *Model) Config() map[string]interface{} {
	m.mu.Lock()
	cfg := make(map[string]interface{}, len(m.details.Config))
	for k, v := range m.details.Config {
		cfg[k] = v
	}
	m.mu.Unlock()
	m.metrics.ModelConfigReads.Inc()
	return cfg
}

// WatchConfig creates a watcher for the model config.
func (m *Model) WatchConfig(keys ...string) *ConfigWatcher {
	return newConfigWatcher(keys, m.hashCache, m.hub, m.topic(modelConfigChange))
}

// Report returns information that is used in the dependency engine report.
func (m *Model) Report() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	return map[string]interface{}{
		"name":              m.details.Owner + "/" + m.details.Name,
		"life":              m.details.Life,
		"application-count": len(m.applications),
		"machine-count":     len(m.machines),
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

// Machine returns the machine with the input id.
// If the machine is not found, a NotFoundError is returned.
func (m *Model) Machine(machineId string) (*Machine, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	machine, found := m.machines[machineId]
	if !found {
		return nil, errors.NotFoundf("machine %q", machineId)
	}
	return machine, nil
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

// updateMachine adds or updates the machine in the model.
func (m *Model) updateMachine(ch MachineChange) {
	m.mu.Lock()

	machine, found := m.machines[ch.Id]
	if !found {
		machine = newMachine(m.metrics, m.hub)
		m.machines[ch.Id] = machine
	}
	machine.setDetails(ch)

	m.mu.Unlock()
}

// removeMachine removes the machine from the model.
func (m *Model) removeMachine(ch RemoveMachine) {
	m.mu.Lock()
	delete(m.machines, ch.Id)
	m.mu.Unlock()
}

// topic prefixes the input string with the model UUID.
func (m *Model) topic(suffix string) string {
	return m.details.ModelUUID + ":" + suffix
}

func (m *Model) setDetails(details ModelChange) {
	m.mu.Lock()

	m.details = details
	hashCache, configHash := newHashCache(details.Config, m.metrics.ModelHashCacheHit, m.metrics.ModelHashCacheMiss)
	if configHash != m.configHash {
		m.configHash = configHash
		m.hashCache = hashCache
		m.hub.Publish(m.topic(modelConfigChange), hashCache)
	}

	m.mu.Unlock()
}
