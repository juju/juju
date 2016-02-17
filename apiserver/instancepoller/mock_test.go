// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"sort"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/instancepoller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// mockState implements StateInterface and allows inspection of called
// methods.
type mockState struct {
	*testing.Stub

	mu sync.Mutex

	configWatchers   []*mockConfigWatcher
	machinesWatchers []*mockMachinesWatcher

	config   *config.Config
	machines map[string]*mockMachine
}

func NewMockState() *mockState {
	return &mockState{
		Stub:     &testing.Stub{},
		machines: make(map[string]*mockMachine),
	}
}

var _ instancepoller.StateInterface = (*mockState)(nil)

// CheckFindEntityCall is a helper wrapper aroud
// testing.Stub.CheckCall for FindEntity.
func (m *mockState) CheckFindEntityCall(c *gc.C, index int, machineId string) {
	m.CheckCall(c, index, "FindEntity", interface{}(names.NewMachineTag(machineId)))
}

// CheckSetProviderAddressesCall is a helper wrapper aroud
// testing.Stub.CheckCall for SetProviderAddresses.
func (m *mockState) CheckSetProviderAddressesCall(c *gc.C, index int, addrs []network.Address) {
	args := make([]interface{}, len(addrs))
	for i, addr := range addrs {
		args[i] = addr
	}
	m.CheckCall(c, index, "SetProviderAddresses", args...)
}

// WatchForModelConfigChanges implements StateInterface.
func (m *mockState) WatchForModelConfigChanges() state.NotifyWatcher {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "WatchForModelConfigChanges")

	w := NewMockConfigWatcher(m.NextErr())
	m.configWatchers = append(m.configWatchers, w)
	return w
}

// ModelConfig implements StateInterface.
func (m *mockState) ModelConfig() (*config.Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "ModelConfig")

	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.config, nil
}

// SetConfig updates the environ config stored internally. Triggers a
// change event for all created config watchers.
func (m *mockState) SetConfig(c *gc.C, newConfig *config.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = newConfig

	// Notify any watchers for the changes.
	for _, w := range m.configWatchers {
		w.incoming <- struct{}{}
	}
}

// WatchModelMachines implements StateInterface.
func (m *mockState) WatchModelMachines() state.StringsWatcher {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "WatchModelMachines")

	ids := make([]string, 0, len(m.machines))
	// Initial event - all machine ids, sorted.
	for id := range m.machines {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	w := NewMockMachinesWatcher(ids, m.NextErr())
	m.machinesWatchers = append(m.machinesWatchers, w)
	return w
}

// FindEntity implements StateInterface.
func (m *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "FindEntity", tag)

	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, errors.NotValidf("nil tag is") // +" not valid"
	}
	machine, found := m.machines[tag.Id()]
	if !found {
		return nil, errors.NotFoundf("machine %s", tag.Id())
	}
	return machine, nil
}

// SetMachineInfo adds a new or updates existing mockMachine info.
// Triggers any created mock machines watchers to return a change.
func (m *mockState) SetMachineInfo(c *gc.C, args machineInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c.Assert(args.id, gc.Not(gc.Equals), "")

	machine, found := m.machines[args.id]
	if !found {
		machine = &mockMachine{
			Stub:        m.Stub, // reuse parent stub.
			machineInfo: args,
		}
	} else {
		machine.machineInfo = args
	}
	m.machines[args.id] = machine

	// Notify any watchers for the changes.
	ids := []string{args.id}
	for _, w := range m.machinesWatchers {
		w.incoming <- ids
	}
}

// RemoveMachine removes an existing mockMachine with the given id.
// Triggers the machines watchers on success. If the id is not found
// no error occurs and no change is reported by the watchers.
func (m *mockState) RemoveMachine(c *gc.C, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, found := m.machines[id]; !found {
		return
	}

	delete(m.machines, id)

	// Notify any watchers for the changes.
	ids := []string{id}
	for _, w := range m.machinesWatchers {
		w.incoming <- ids
	}
}

// Machine implements StateInterface.
func (m *mockState) Machine(id string) (instancepoller.StateMachine, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "Machine", id)

	if err := m.NextErr(); err != nil {
		return nil, err
	}
	machine, found := m.machines[id]
	if !found {
		return nil, errors.NotFoundf("machine %s", id)
	}
	return machine, nil
}

// StartSync implements statetesting.SyncStarter, so mockState can be
// used with watcher helpers/checkers.
func (m *mockState) StartSync() {}

type machineInfo struct {
	id                string
	instanceId        instance.Id
	status            status.StatusInfo
	instanceStatus    status.StatusInfo
	providerAddresses []network.Address
	life              state.Life
	isManual          bool
}

type mockMachine struct {
	*testing.Stub
	instancepoller.StateMachine

	mu sync.Mutex

	machineInfo
}

var _ instancepoller.StateMachine = (*mockMachine)(nil)

// InstanceId implements StateMachine.
func (m *mockMachine) InstanceId() (instance.Id, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "InstanceId")
	if err := m.NextErr(); err != nil {
		return "", err
	}
	return m.instanceId, nil
}

// ProviderAddresses implements StateMachine.
func (m *mockMachine) ProviderAddresses() []network.Address {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "ProviderAddresses")
	m.NextErr() // consume the unused error
	return m.providerAddresses
}

// SetProviderAddresses implements StateMachine.
func (m *mockMachine) SetProviderAddresses(addrs ...network.Address) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	args := make([]interface{}, len(addrs))
	for i, addr := range addrs {
		args[i] = addr
	}
	m.MethodCall(m, "SetProviderAddresses", args...)
	if err := m.NextErr(); err != nil {
		return err
	}
	m.providerAddresses = addrs
	return nil
}

// InstanceStatus implements StateMachine.
func (m *mockMachine) InstanceStatus() (status.StatusInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "InstanceStatus")
	if err := m.NextErr(); err != nil {
		return status.StatusInfo{}, err
	}
	return m.instanceStatus, nil
}

// SetInstanceStatus implements StateMachine.
func (m *mockMachine) SetInstanceStatus(instanceStatus status.Status, info string, data map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "SetInstanceStatus", instanceStatus)
	if err := m.NextErr(); err != nil {
		return err
	}
	m.instanceStatus = status.StatusInfo{
		Status:  instanceStatus,
		Message: info,
		Data:    data,
	}
	return nil
}

// Life implements StateMachine.
func (m *mockMachine) Life() state.Life {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "Life")
	m.NextErr() // consume the unused error
	return m.life
}

// IsManual implements StateMachine.
func (m *mockMachine) IsManual() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "IsManual")
	return m.isManual, m.NextErr()
}

// Status implements StateMachine.
func (m *mockMachine) Status() (status.StatusInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.MethodCall(m, "Status")
	return m.status, m.NextErr()
}

type mockBaseWatcher struct {
	err error

	closeChanges func()
	done         chan struct{}
}

var _ state.Watcher = (*mockBaseWatcher)(nil)

func NewMockBaseWatcher(err error, closeChanges func()) *mockBaseWatcher {
	w := &mockBaseWatcher{
		err:          err,
		closeChanges: closeChanges,
		done:         make(chan struct{}),
	}
	if err != nil {
		// Don't start the loop if we should fail.
		w.Stop()
	}
	return w
}

// Kill implements state.Watcher.
func (m *mockBaseWatcher) Kill() {}

// Stop implements state.Watcher.
func (m *mockBaseWatcher) Stop() error {
	select {
	case <-m.done:
		// already closed
	default:
		// Signal the loop we want to stop.
		close(m.done)
		// Signal the clients we've closed.
		m.closeChanges()
	}
	return m.err
}

// Wait implements state.Watcher.
func (m *mockBaseWatcher) Wait() error {
	return m.Stop()
}

// Err implements state.Watcher.
func (m *mockBaseWatcher) Err() error {
	return m.err
}

type mockConfigWatcher struct {
	*mockBaseWatcher

	incoming chan struct{}
	changes  chan struct{}
}

var _ state.NotifyWatcher = (*mockConfigWatcher)(nil)

func NewMockConfigWatcher(err error) *mockConfigWatcher {
	changes := make(chan struct{})
	w := &mockConfigWatcher{
		changes:         changes,
		incoming:        make(chan struct{}),
		mockBaseWatcher: NewMockBaseWatcher(err, func() { close(changes) }),
	}
	if err == nil {
		go w.loop()
	}
	return w
}

func (m *mockConfigWatcher) loop() {
	// Prepare initial event.
	outChanges := m.changes
	// Forward any incoming changes until stopped.
	for {
		select {
		case <-m.done:
			// We're about to quit.
			return
		case outChanges <- struct{}{}:
			outChanges = nil
		case <-m.incoming:
			outChanges = m.changes
		}
	}
}

// Changes implements state.NotifyWatcher.
func (m *mockConfigWatcher) Changes() <-chan struct{} {
	return m.changes
}

type mockMachinesWatcher struct {
	*mockBaseWatcher

	initial  []string
	incoming chan []string
	changes  chan []string
}

func NewMockMachinesWatcher(initial []string, err error) *mockMachinesWatcher {
	changes := make(chan []string)
	w := &mockMachinesWatcher{
		initial:         initial,
		changes:         changes,
		incoming:        make(chan []string),
		mockBaseWatcher: NewMockBaseWatcher(err, func() { close(changes) }),
	}
	if err == nil {
		go w.loop()
	}
	return w
}

func (m *mockMachinesWatcher) loop() {
	// Prepare initial event.
	unsent := m.initial
	outChanges := m.changes
	// Forward any incoming changes until stopped.
	for {
		select {
		case <-m.done:
			// We're about to quit.
			return
		case outChanges <- unsent:
			outChanges = nil
			unsent = nil
		case ids := <-m.incoming:
			unsent = append(unsent, ids...)
			outChanges = m.changes
		}
	}
}

// Changes implements state.StringsWatcher.
func (m *mockMachinesWatcher) Changes() <-chan []string {
	return m.changes
}

var _ state.StringsWatcher = (*mockMachinesWatcher)(nil)
