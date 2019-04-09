// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/errors"
	"github.com/juju/pubsub"
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

func newModel(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Model {
	m := &Model{
		metrics: metrics,
		// TODO: consider a separate hub per model for better scalability
		// when many models.
		hub:          hub,
		applications: make(map[string]*Application),
		charms:       make(map[string]*Charm),
		machines:     make(map[string]*Machine),
		units:        make(map[string]*Unit),
	}
	// wire up the removalDelta so that the entity can collate all the deltas
	// during a sweep phase. If this isn't correctly wired up, an error will be
	// returned during the sweeping phase.
	m.entity.removalDelta = m.removalDelta
	return m
}

// Model is a cached model in the controller. The model is kept up to
// date with changes flowing into the cached controller.
type Model struct {
	entity

	metrics *ControllerGauges
	hub     *pubsub.SimpleHub

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
	w := newConfigWatcher(keys, m.hashCache, m.hub, m.topic(modelConfigChange))
	return w
}

// Report returns information that is used in the dependency engine report.
func (m *Model) Report() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

	app, found := m.applications[appName]
	if !found {
		return nil, errors.NotFoundf("application %q", appName)
	}
	return app, nil
}

// Charm returns the charm for the input charmURL.
// If the charm is not found, a NotFoundError is returned.
func (m *Model) Charm(charmURL string) (*Charm, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	charm, found := m.charms[charmURL]
	if !found {
		return nil, errors.NotFoundf("charm %q", charmURL)
	}
	return charm, nil
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

// WatchMachines returns a PredicateStringsWatcher to notify about
// added and removed machines in the model.  The initial event contains
// a slice of the current machine ids.
func (m *Model) WatchMachines() *PredicateStringsWatcher {
	m.mu.Lock()

	// Gather initial slice of machines in this model.
	machines := make([]string, 0, len(m.machines))
	for id := range m.machines {
		machines = append(machines, id)
	}

	w := newChangeWatcher(machines...)
	unsub := m.hub.Subscribe(m.topic(modelAddRemoveMachine), w.changed)

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		return nil
	})

	m.mu.Unlock()
	return w
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

func (m *Model) mark() int {
	// Result is set to 1, as the model also needs to be marked, so that's our
	// initial state.
	m.entity.mark()
	result := 1

	m.mu.Lock()
	for _, app := range m.applications {
		app.mark()
		result++
	}
	for _, charm := range m.charms {
		charm.mark()
		result++
	}
	for _, machine := range m.machines {
		machine.mark()
		result++
	}
	for _, unit := range m.units {
		unit.mark()
		result++
	}
	m.mu.Unlock()
	return result
}

func (m *Model) sweep() (*SweepDeltas, error) {
	deltas := &SweepDeltas{}
	m.mu.Lock()

	// Walk through the model itself, so we can safely remove the model.
	if m.freshness == stale {
		deltas.Merge(&SweepDeltas{
			Deltas: []interface{}{m.removalDelta()},
		})
	} else {
		deltas.Merge(&SweepDeltas{
			FreshCount: 1,
		})
	}

	// Go through and sweep all the sub-related entities.
	for _, app := range m.applications {
		appDelta, err := app.sweep()
		if err != nil {
			return nil, errors.Trace(err)
		}
		deltas.Merge(appDelta)
	}
	for _, charm := range m.charms {
		charmDelta, err := charm.sweep()
		if err != nil {
			return nil, errors.Trace(err)
		}
		deltas.Merge(charmDelta)
	}
	for _, machine := range m.machines {
		machineDelta, err := machine.sweep()
		if err != nil {
			return nil, errors.Trace(err)
		}
		deltas.Merge(machineDelta)
	}
	for _, unit := range m.units {
		unitDelta, err := unit.sweep()
		if err != nil {
			return nil, errors.Trace(err)
		}
		deltas.Merge(unitDelta)
	}
	m.mu.Unlock()
	return deltas, nil
}

// removalDelta returns a delta that is required to remove the Model. If this
// is not correctly wired up when setting up the Model, then a error will be
// returned stating this fact when the Sweep phase of the GC.
func (m *Model) removalDelta() interface{} {
	return RemoveModel{
		ModelUUID: m.details.ModelUUID,
	}
}

// remove the other entities associated with the model, so everything is cleanly
// cleaned up
func (m *Model) remove() {
	m.mu.Lock()
	for key, app := range m.applications {
		app.remove()
		delete(m.applications, key)
	}
	for key, charm := range m.charms {
		charm.remove()
		delete(m.charms, key)
	}
	for key, machine := range m.machines {
		machine.remove()
		delete(m.machines, key)
	}
	for key, unit := range m.units {
		unit.remove()
		delete(m.units, key)
	}
	m.mu.Unlock()
}

// updateApplication adds or updates the application in the model.
func (m *Model) updateApplication(ch ApplicationChange) {
	m.mu.Lock()
	m.freshness = fresh

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
	if _, ok := m.applications[ch.Name]; ok {
		// TODO (stickupkid): ensure we clean up the application, so that it
		// also cleans up the watchers
		delete(m.applications, ch.Name)
	}
	m.mu.Unlock()
}

// updateCharm adds or updates the charm in the model.
func (m *Model) updateCharm(ch CharmChange) {
	m.mu.Lock()
	m.freshness = fresh

	charm, found := m.charms[ch.CharmURL]
	if !found {
		charm = newCharm(m.metrics, m.hub)
		m.charms[ch.CharmURL] = charm
	}
	charm.setDetails(ch)

	m.mu.Unlock()
}

// removeCharm removes the charm from the model.
func (m *Model) removeCharm(ch RemoveCharm) {
	m.mu.Lock()
	if _, ok := m.charms[ch.CharmURL]; ok {
		// TODO (stickupkid): ensure we clean up the charm, so that it
		// also cleans up the watchers
		delete(m.charms, ch.CharmURL)
	}
	m.mu.Unlock()
}

// updateUnit adds or updates the unit in the model.
func (m *Model) updateUnit(ch UnitChange) {
	m.mu.Lock()
	m.freshness = fresh

	unit, found := m.units[ch.Name]
	if !found {
		unit = newUnit(m.metrics, m.hub)
		m.units[ch.Name] = unit
		m.hub.Publish(m.topic(modelUnitLXDProfileChange), unit)
	}
	unit.setDetails(ch)

	m.mu.Unlock()
}

// removeUnit removes the unit from the model.
func (m *Model) removeUnit(ch RemoveUnit) {
	m.mu.Lock()
	if unit, ok := m.units[ch.Name]; ok {
		// TODO (stickupkid): ensure we clean up the unit, so that it
		// also cleans up the watchers
		delete(m.units, ch.Name)
		m.hub.Publish(m.topic(modelUnitLXDProfileChange), []string{ch.Name, unit.details.Application})
	}
	m.mu.Unlock()
}

// updateMachine adds or updates the machine in the model.
func (m *Model) updateMachine(ch MachineChange) {
	m.mu.Lock()
	m.freshness = fresh

	machine, found := m.machines[ch.Id]
	if !found {
		machine = newMachine(m)
		m.machines[ch.Id] = machine
		m.hub.Publish(m.topic(modelAddRemoveMachine), []string{ch.Id})
	}
	machine.setDetails(ch)

	m.mu.Unlock()
}

// removeMachine removes the machine from the model.
func (m *Model) removeMachine(ch RemoveMachine) {
	m.mu.Lock()
	if _, ok := m.machines[ch.Id]; ok {
		// TODO (stickupkid): ensure we clean up the machine, so that it
		// also cleans up the watchers
		delete(m.machines, ch.Id)
		m.hub.Publish(m.topic(modelAddRemoveMachine), []string{ch.Id})
	}
	m.mu.Unlock()
}

// topic prefixes the input string with the model UUID.
func (m *Model) topic(suffix string) string {
	return modelTopic(m.details.ModelUUID, suffix)
}

func modelTopic(modeluuid, suffix string) string {
	return modeluuid + ":" + suffix
}

func (m *Model) setDetails(details ModelChange) {
	m.mu.Lock()

	m.freshness = fresh
	m.details = details
	hashCache, configHash := newHashCache(details.Config, m.metrics.ModelHashCacheHit, m.metrics.ModelHashCacheMiss)
	if configHash != m.configHash {
		m.configHash = configHash
		m.hashCache = hashCache
		m.hub.Publish(m.topic(modelConfigChange), hashCache)
	}

	m.mu.Unlock()
}
