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
	// Model config has changed.
	modelConfigChange = "model-config-change"
	// A machine has been added to, or removed from the model.
	modelAddRemoveMachine = "model-add-remove-machine"
	// A unit has landed on a machine, or a subordinate unit has been changed,
	// Either of which likely indicate the addition of a unit to the model.
	modelUnitAdd = "model-unit-add"
	// A unit has been removed from the model.
	modelUnitRemove = "model-unit-remove"
	// A branch has been removed from the model.
	modelBranchRemove = "model-branch-remove"
)

func newModel(metrics *ControllerGauges, hub *pubsub.SimpleHub, res *Resident) *Model {
	m := &Model{
		Resident:     res,
		metrics:      metrics,
		hub:          hub,
		applications: make(map[string]*Application),
		charms:       make(map[string]*Charm),
		machines:     make(map[string]*Machine),
		units:        make(map[string]*Unit),
		branches:     make(map[string]*Branch),
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
	branches     map[string]*Branch
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

// UUID returns the model's model-uuid.
func (m *Model) UUID() string {
	defer m.doLocked()()
	return m.details.ModelUUID
}

// Name returns the current model's name.
func (m *Model) Name() string {
	defer m.doLocked()()
	return m.details.Name
}

// WatchConfig creates a watcher for the model config.
func (m *Model) WatchConfig(keys ...string) *ConfigWatcher {
	return newConfigWatcher(keys, m.hashCache, m.hub, modelConfigChange, m.Resident)
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
		"branch-count":      len(m.branches),
	}
}

// Branches returns all active branches in the model.
func (m *Model) Branches() []Branch {
	m.mu.Lock()

	branches := make([]Branch, len(m.branches))
	i := 0
	for _, b := range m.branches {
		branches[i] = b.copy()
		i += 1
	}

	m.mu.Unlock()
	return branches
}

// Branch returns the branch with the input name.
// If the branch is not found, a NotFoundError is returned.
// All API-level logic identifies active branches by their name whereas they
// are managed in the cache by ID - we iterate over the map to locate them.
// We do not expect many active branches to exist at once,
// so the performance should be acceptable.
func (m *Model) Branch(name string) (Branch, error) {
	defer m.doLocked()()

	for _, b := range m.branches {
		if b.details.Name == name {
			return b.copy(), nil
		}
	}
	return Branch{}, errors.NotFoundf("branch %q", name)
}

// Application returns the application for the input name.
// If the application is not found, a NotFoundError is returned.
func (m *Model) Application(appName string) (Application, error) {
	defer m.doLocked()()

	app, found := m.applications[appName]
	if !found {
		return Application{}, errors.NotFoundf("application %q", appName)
	}
	return app.copy(), nil
}

// Units returns all units in the model.
func (m *Model) Units() map[string]Unit {
	m.mu.Lock()

	units := make(map[string]Unit, len(m.units))
	for name, u := range m.units {
		units[name] = u.copy()
	}

	m.mu.Unlock()
	return units
}

// Unit returns the unit with the input name.
// If the unit is not found, a NotFoundError is returned.
func (m *Model) Unit(unitName string) (Unit, error) {
	defer m.doLocked()()

	unit, found := m.units[unitName]
	if !found {
		return Unit{}, errors.NotFoundf("unit %q", unitName)
	}
	return unit.copy(), nil
}

// Machines makes a copy of the model's machine collection and returns it.
func (m *Model) Machines() map[string]Machine {
	m.mu.Lock()

	machines := make(map[string]Machine, len(m.machines))
	for k, v := range m.machines {
		machines[k] = v.copy()
	}

	m.mu.Unlock()
	return machines
}

// Machine returns the machine with the input id.
// If the machine is not found, a NotFoundError is returned.
func (m *Model) Machine(machineId string) (Machine, error) {
	defer m.doLocked()()

	machine, found := m.machines[machineId]
	if !found {
		return Machine{}, errors.NotFoundf("machine %q", machineId)
	}
	return machine.copy(), nil
}

// Charm returns the charm for the input charmURL.
// If the charm is not found, a NotFoundError is returned.
func (m *Model) Charm(charmURL string) (Charm, error) {
	defer m.doLocked()()

	charm, found := m.charms[charmURL]
	if !found {
		return Charm{}, errors.NotFoundf("charm %q", charmURL)
	}
	return charm.copy(), nil
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
	unsub := m.hub.Subscribe(modelAddRemoveMachine, w.changed)

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		deregister()
		return nil
	})

	return w, nil
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
		unit = newUnit(m, rm.new())
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
		m.hub.Publish(modelUnitRemove, unit.copy())
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
		m.hub.Publish(modelAddRemoveMachine, []string{ch.Id})
	}
	machine.setDetails(ch)

	m.mu.Unlock()
}

// removeMachine removes the machine from the model.
func (m *Model) removeMachine(ch RemoveMachine) error {
	defer m.doLocked()()

	machine, ok := m.machines[ch.Id]
	if ok {
		m.hub.Publish(modelAddRemoveMachine, []string{ch.Id})
		if err := machine.evict(); err != nil {
			return errors.Trace(err)
		}
		delete(m.machines, ch.Id)
	}
	return nil
}

// updateBranch adds or updates the branch in the model.
// Only "in-flight" branches should ever reside in the change.
// A committed or aborted branch (with a non-zero time-stamp for completion)
// should be passed through by the cache worker as a deletion.
func (m *Model) updateBranch(ch BranchChange, rm *residentManager) {
	m.mu.Lock()

	branch, found := m.branches[ch.Id]
	if !found {
		branch = newBranch(m.metrics, m.hub, rm.new())
		m.branches[ch.Id] = branch
	}
	branch.setDetails(ch)

	m.mu.Unlock()
}

// removeBranch removes the branch from the model.
func (m *Model) removeBranch(ch RemoveBranch) error {
	defer m.doLocked()()

	branch, ok := m.branches[ch.Id]
	if ok {
		m.hub.Publish(modelBranchRemove, branch.Name())
		if err := branch.evict(); err != nil {
			return errors.Trace(err)
		}
		delete(m.branches, ch.Id)
	}
	return nil
}

func (m *Model) setDetails(details ModelChange) {
	m.mu.Lock()

	// If this is the first receipt of details, set the removal message.
	if m.removalMessage == nil {
		m.removalMessage = RemoveModel{
			ModelUUID: details.ModelUUID,
		}
	}

	m.setStale(false)
	m.details = details

	hashCache, configHash := newHashCache(details.Config, m.metrics.ModelHashCacheHit, m.metrics.ModelHashCacheMiss)
	if configHash != m.configHash {
		m.configHash = configHash
		m.hashCache = hashCache
		m.hashCache.incMisses()
		m.hub.Publish(modelConfigChange, hashCache)
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
