// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"gopkg.in/juju/names.v2"
)

const (
	// a machine has been added or removed from the model.
	modelAddRemoveMachine = "model-add-remove-machine"
	// model config has changed.
	modelConfigChange = "model-config-change"
	// a unit in the model has been changed such than a lxd profile change
	// maybe be necessary has been made.
	modelUnitLXDProfileChange = "model-unit-lxd-profile-change"
)

func newModel(metrics *ControllerGauges, hub *pubsub.SimpleHub, res *Resident) *Model {
	m := &Model{
		Resident: res,
		metrics:  metrics,
		// TODO: consider a separate hub per model for better scalability
		// when many models.
		hub:          hub,
		applications: make(map[string]*Application),
		charms:       make(map[string]*Charm),
		machines:     make(map[string]*Machine),
		units:        make(map[string]*Unit),
	}
	return m
}

// Model is a cached model in the controller. The model is kept up to
// date with changes flowing into the cached controller.
type Model struct {
	// Resident identifies the model as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details      ModelChange
	configHash   string
	hashCache    *hashCache
	applications map[string]*Application
	charms       map[string]*Charm
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

// Name returns the current model's name.
func (m *Model) Name() string {
	return m.details.Name
}

// WatchConfig creates a watcher for the model config.
func (m *Model) WatchConfig(keys ...string) *ConfigWatcher {
	return newConfigWatcher(keys, m.hashCache, m.hub, m.topic(modelConfigChange), m.Resident)
}

// Report returns information that is used in the dependency engine report.
func (m *Model) Report() map[string]interface{} {
	defer m.doLocked()()

	return map[string]interface{}{
		"name":              m.details.Owner + "/" + m.details.Name,
		"life":              m.details.Life,
		"application-count": len(m.applications),
		"charm-count":       len(m.charms),
		"machine-count":     len(m.machines),
		"unit-count":        len(m.units),
	}
}

// Application returns the application for the input name.
// If the application is not found, a NotFoundError is returned.
func (m *Model) Application(appName string) (*Application, error) {
	defer m.doLocked()()

	app, found := m.applications[appName]
	if !found {
		return nil, errors.NotFoundf("application %q", appName)
	}
	return app, nil
}

// Charm returns the charm for the input charmURL.
// If the charm is not found, a NotFoundError is returned.
func (m *Model) Charm(charmURL string) (*Charm, error) {
	defer m.doLocked()()

	charm, found := m.charms[charmURL]
	if !found {
		return nil, errors.NotFoundf("charm %q", charmURL)
	}
	return charm, nil
}

// Machines makes a copy of the model's machine collection and returns it.
func (m *Model) Machines() map[string]*Machine {
	machines := make(map[string]*Machine)

	m.mu.Lock()
	for k, v := range m.machines {
		machines[k] = v
	}
	m.mu.Unlock()

	return machines
}

// Machine returns the machine with the input id.
// If the machine is not found, a NotFoundError is returned.
func (m *Model) Machine(machineId string) (*Machine, error) {
	defer m.doLocked()()

	machine, found := m.machines[machineId]
	if !found {
		return nil, errors.NotFoundf("machine %q", machineId)
	}
	return machine, nil
}

// WatchMachines returns a PredicateStringsWatcher to notify about
// added and removed machines in the model.  The initial event contains
// a slice of the current machine ids.  Containers are excluded.
func (m *Model) WatchMachines() (*PredicateStringsWatcher, error) {
	defer m.doLocked()()

	// Create a compiled regexp to match machines not containers.
	compiled, err := m.machineRegexp()
	if err != nil {
		return nil, err
	}
	fn := regexpPredicate(compiled)

	// Gather initial slice of machines in this model.
	machines := make([]string, 0)
	for k := range m.machines {
		if fn(k) {
			machines = append(machines, k)
		}
	}

	w := newPredicateStringsWatcher(fn, machines...)
	deregister := m.registerWorker(w)
	unsub := m.hub.Subscribe(m.topic(modelAddRemoveMachine), w.changed)

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		deregister()
		return nil
	})

	return w, nil
}

// Unit returns the unit with the input name.
// If the unit is not found, a NotFoundError is returned.
func (m *Model) Unit(unitName string) (*Unit, error) {
	defer m.doLocked()()

	unit, found := m.units[unitName]
	if !found {
		return nil, errors.NotFoundf("unit %q", unitName)
	}
	return unit, nil
}

// updateApplication adds or updates the application in the model.
func (m *Model) updateApplication(ch ApplicationChange, rm *residentManager) {
	m.mu.Lock()

	app, found := m.applications[ch.Name]
	if !found {
		app = newApplication(m.metrics, m.hub, rm.new())
		m.applications[ch.Name] = app
	}
	app.setDetails(ch)

	m.mu.Unlock()
}

// removeApplication removes the application from the model.
func (m *Model) removeApplication(ch RemoveApplication) error {
	defer m.doLocked()()

	app, ok := m.applications[ch.Name]
	if ok {
		if err := app.evict(); err != nil {
			return errors.Trace(err)
		}
		delete(m.applications, ch.Name)
	}
	return nil
}

// updateCharm adds or updates the charm in the model.
func (m *Model) updateCharm(ch CharmChange, rm *residentManager) {
	m.mu.Lock()

	charm, found := m.charms[ch.CharmURL]
	if !found {
		charm = newCharm(m.metrics, m.hub, rm.new())
		m.charms[ch.CharmURL] = charm
	}
	charm.setDetails(ch)

	m.mu.Unlock()
}

// removeCharm removes the charm from the model.
func (m *Model) removeCharm(ch RemoveCharm) error {
	defer m.doLocked()()

	charm, ok := m.charms[ch.CharmURL]
	if ok {
		if err := charm.evict(); err != nil {
			return errors.Trace(err)
		}
		delete(m.charms, ch.CharmURL)
	}
	return nil
}

// updateUnit adds or updates the unit in the model.
func (m *Model) updateUnit(ch UnitChange, rm *residentManager) {
	m.mu.Lock()

	unit, found := m.units[ch.Name]
	if !found {
		unit = newUnit(m.metrics, m.hub, rm.new())
		m.units[ch.Name] = unit
	}
	unit.setDetails(ch)

	m.mu.Unlock()
}

// removeUnit removes the unit from the model.
func (m *Model) removeUnit(ch RemoveUnit) error {
	defer m.doLocked()()

	unit, ok := m.units[ch.Name]
	if ok {
		m.hub.Publish(m.topic(modelUnitLXDProfileChange), []string{ch.Name, unit.details.Application})
		if err := unit.evict(); err != nil {
			return errors.Trace(err)
		}
		delete(m.units, ch.Name)
	}
	return nil
}

// updateMachine adds or updates the machine in the model.
func (m *Model) updateMachine(ch MachineChange, rm *residentManager) {
	m.mu.Lock()

	machine, found := m.machines[ch.Id]
	if !found {
		machine = newMachine(m, rm.new())
		m.machines[ch.Id] = machine
		m.hub.Publish(m.topic(modelAddRemoveMachine), []string{ch.Id})
	}
	machine.setDetails(ch)

	m.mu.Unlock()
}

// removeMachine removes the machine from the model.
func (m *Model) removeMachine(ch RemoveMachine) error {
	defer m.doLocked()()

	machine, ok := m.machines[ch.Id]
	if ok {
		m.hub.Publish(m.topic(modelAddRemoveMachine), []string{ch.Id})
		if err := machine.evict(); err != nil {
			return errors.Trace(err)
		}
		delete(m.machines, ch.Id)
	}
	return nil
}

// topic prefixes the input string with the model UUID.
func (m *Model) topic(suffix string) string {
	return modelTopic(m.details.ModelUUID, suffix)
}

func modelTopic(modelUUID, suffix string) string {
	return modelUUID + ":" + suffix
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

func (m *Model) machineRegexp() (*regexp.Regexp, error) {
	regExp := fmt.Sprintf("^%s$", names.NumberSnippet)
	return regexp.Compile(regExp)
}

func (m *Model) doLocked() func() {
	m.mu.Lock()
	return m.mu.Unlock
}
