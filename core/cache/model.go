// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
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

type modelConfig struct {
	initializing func() bool
	metrics      *ControllerGauges
	hub          *pubsub.SimpleHub
	chub         *pubsub.SimpleHub
	res          *Resident
}

func newModel(config modelConfig) *Model {
	m := &Model{
		initializing:  config.initializing,
		Resident:      config.res,
		metrics:       config.metrics,
		hub:           config.hub,
		controllerHub: config.chub,
		applications:  make(map[string]*Application),
		charms:        make(map[string]*Charm),
		machines:      make(map[string]*Machine),
		units:         make(map[string]*Unit),
		relations:     make(map[string]*Relation),
		branches:      make(map[string]*Branch),
	}
	return m
}

// Model is a cached model in the controller. The model is kept up to
// date with changes flowing into the cached controller.
type Model struct {
	// Resident identifies the model as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	initializing  func() bool
	metrics       *ControllerGauges
	hub           *pubsub.SimpleHub
	controllerHub *pubsub.SimpleHub
	mu            sync.Mutex

	details     ModelChange
	summary     ModelSummary
	summaryHash string

	configHash   string
	hashCache    *hashCache
	applications map[string]*Application
	charms       map[string]*Charm
	machines     map[string]*Machine
	units        map[string]*Unit
	relations    map[string]*Relation
	branches     map[string]*Branch

	// lastSummaryPublish is here for testing purposes to ensure
	// synchronisation between the test and the handling of the
	// published summary event. This channel is returned by the pubsub
	// hub from the publish call. The channel is closed when all the
	// subscribers have handled the call. We only record the last one
	// as the tests want to know that a set of changes have been applied.
	lastSummaryPublish <-chan struct{}
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

// Summary returns a copy of the current summary, and its hash.
func (m *Model) Summary() (ModelSummary, string) {
	defer m.doLocked()()
	return m.summaryCopy(), m.summaryHash
}

func (m *Model) summaryCopy() ModelSummary {
	result := m.summary
	// Make a copy of the admins slice.
	result.Admins = append([]string(nil), result.Admins...)
	// Make a copy of the messages slice.
	result.Messages = append([]ModelSummaryMessage(nil), result.Messages...)
	return result
}

func (m *Model) visibleTo(user string) bool {
	if user == "" {
		return true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Any permission is sufficient for the user to see the model.
	// Read, Write, or Admin are all good for us. If the user doesn't
	// have any access, they can't see the model.
	_, found := m.details.UserPermissions[user]
	return found
}

// WatchConfig creates a watcher for the model config.
func (m *Model) WatchConfig(keys ...string) *ConfigWatcher {
	m.mu.Lock()
	defer m.mu.Unlock()

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
		"relation-count":    len(m.relations),
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
		i++
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

// Applications makes a copy of the model's application collection and returns it.
func (m *Model) Applications() map[string]Application {
	m.mu.Lock()

	apps := make(map[string]Application, len(m.applications))
	for k, v := range m.applications {
		apps[k] = v.copy()
	}

	m.mu.Unlock()
	return apps
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

// Metrics returns the metrics of the model
func (m *Model) Metrics() *ControllerGauges {
	defer m.doLocked()()
	return m.metrics
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
func (m *Model) Machine(machineID string) (Machine, error) {
	defer m.doLocked()()

	machine, found := m.machines[machineID]
	if !found {
		return Machine{}, errors.NotFoundf("machine %q", machineID)
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
	m.updateSummary()
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
	m.updateSummary()
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

	m.updateSummary()
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
	m.updateSummary()
	return nil
}

// Relation returns the relation with the specified key.
// If the relation is not found, a NotFoundError is returned.
func (m *Model) Relation(key string) (Relation, error) {
	defer m.doLocked()()

	relation, found := m.relations[key]
	if !found {
		return Relation{}, errors.NotFoundf("relation %q", key)
	}
	return relation.copy(), nil
}

// Relations returns all relations in the model.
func (m *Model) Relations() map[string]Relation {
	m.mu.Lock()

	relations := make(map[string]Relation, len(m.relations))
	for key, r := range m.relations {
		relations[key] = r.copy()
	}

	m.mu.Unlock()
	return relations
}

// updateRelation adds or updates the relation in the model.
func (m *Model) updateRelation(ch RelationChange, rm *residentManager) {
	m.mu.Lock()

	relation, found := m.relations[ch.Key]
	if !found {
		relation = newRelation(m, rm.new())
		m.relations[ch.Key] = relation
	}
	relation.setDetails(ch)

	m.mu.Unlock()
}

// removeRelation removes the relation from the model.
func (m *Model) removeRelation(ch RemoveRelation) error {
	defer m.doLocked()()

	relation, ok := m.relations[ch.Key]
	if ok {
		if err := relation.evict(); err != nil {
			return errors.Trace(err)
		}
		delete(m.relations, ch.Key)
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

	m.updateSummary()
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
	m.updateSummary()
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

	m.updateSummary()
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

func unitChangeTopic(source string) string {
	return "unit-change." + source
}

// WaitForUnit is the second attempt at providing a genericish way to wait for
// the cache to be updated. The method subscribes to the hub with the topic
// "unit-change.<source>". The expected payload is a *Unit. The method
// effectively runs another goroutine that will close the result channel when
// the field is updated to the expected value, or the cancel channel is
// signalled. This method is not responsible for checking the current value, it
// only deals with changes.
func (m *Model) WaitForUnit(name string, predicate func(*Unit) bool, cancel <-chan struct{}) <-chan struct{} {
	result := make(chan struct{})

	wait := &waitUnitChange{
		predicate: predicate,
		done:      result,
	}
	// Lock the change so we don't get an event before we record the unsubscriber.
	wait.mu.Lock()
	// The closure that is created below captures the unsub function pointer by reference
	// allowing the function closure to unsubscribe from the hub.
	wait.unsub = m.hub.Subscribe(unitChangeTopic(name), wait.onChange)
	wait.mu.Unlock()
	go wait.loop(cancel)

	// Do the check now, just in case we are already good.
	if unit, err := m.Unit(name); err == nil {
		if predicate(&unit) {
			wait.mu.Lock()
			wait.close()
			wait.mu.Unlock()
		}
	}

	return result
}

type waitUnitChange struct {
	mu        sync.Mutex
	unsub     func()
	predicate func(*Unit) bool
	done      chan struct{}
}

func (w *waitUnitChange) onChange(_ string, payload interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	unit, ok := payload.(*Unit)
	if !ok {
		logger.Criticalf("programming error, payload type incorrect %T", payload)
		return
	}
	if w.predicate(unit) {
		w.close()
	}
}

func (w *waitUnitChange) loop(cancel <-chan struct{}) {
	select {
	case <-cancel:
		w.mu.Lock()
		w.close()
		w.mu.Unlock()
	case <-w.done:
		// Close already handled, so we're done.
	}
}

// close unsubscribes and closes the done channel.
// Due to the race potentials, both are checked prior to action.
// The mutex is acquired outside this method.
func (w *waitUnitChange) close() {
	if w.unsub != nil {
		w.unsub()
		w.unsub = nil
	}
	select {
	case <-w.done:
		// already closed
	default:
		close(w.done)
	}
}

// updateSummary generates a summary of the model,
// then publishes it via the controller's hub.
// Callers of this method must take responsibility for appropriate locking.
func (m *Model) updateSummary() {
	if m.initializing() {
		logger.Tracef("skipping update - cache is initializing")
		return
	}

	overallStatus := StatusGreen
	var messages []ModelSummaryMessage
	var machines, containers int

	for id, machine := range m.machines {
		if names.IsContainerMachine(id) {
			containers++
		} else {
			machines++
		}

		// What machine statuses do we care about?
		if st := machine.details.AgentStatus; st.Status == status.Error {
			overallStatus = StatusRed
			messages = append(messages, ModelSummaryMessage{
				Agent:   id,
				Message: st.Message,
			})
		}
	}

	for id, unit := range m.units {
		if st := unit.details.AgentStatus; st.Status == status.Error {
			overallStatus = StatusRed
			messages = append(messages, ModelSummaryMessage{
				Agent:   id,
				Message: st.Message,
			})
		} else if st := unit.details.WorkloadStatus; st.Status == status.Blocked {
			if overallStatus == StatusGreen {
				overallStatus = StatusYellow
			}
			messages = append(messages, ModelSummaryMessage{
				Agent:   id,
				Message: st.Message,
			})
		}
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].Agent < messages[j].Agent })

	var admins []string
	for user, access := range m.details.UserPermissions {
		if access == permission.AdminAccess {
			admins = append(admins, user)
		}
	}
	sort.Strings(admins)

	summary := ModelSummary{
		UUID:        m.details.ModelUUID,
		Namespace:   m.details.Owner,
		Name:        m.details.Name,
		Admins:      admins,
		Status:      overallStatus,
		Annotations: copyStringMap(m.details.Annotations),
		Messages:    messages,

		Cloud:      m.details.Cloud,
		Region:     m.details.CloudRegion,
		Credential: m.details.CloudCredential,

		MachineCount:     machines,
		ContainerCount:   containers,
		ApplicationCount: len(m.applications),
		UnitCount:        len(m.units),
		RelationCount:    len(m.relations),
	}
	m.summary = summary

	hash, err := summary.hash()
	if err != nil {
		logger.Errorf("unable to generate hash for summary: %s", err)
	}
	logger.Tracef("summary hash, was %q, now %q", m.summaryHash, hash)
	// In order to accurately deal with permission changes, and send an appropriate
	// summary in the situation where a new user can see a model, we need to always
	// publish summary updated topics with the hash, and let the watchers themselves
	// track the last hash they sent.
	payload := modelSummaryPayload{
		summary:   m.summaryCopy(),
		visibleTo: m.visibleTo,
		hash:      hash,
	}
	m.lastSummaryPublish = m.controllerHub.Publish(modelSummaryUpdatedTopic, payload)
	m.summaryHash = hash
}
