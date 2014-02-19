package peergrouper

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils/voyeur"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
)

// This file holds helper functions for mocking pieces of State and replicaset
// that we don't want to directly depend on in unit tests.

type fakeState struct {
	mu           sync.Mutex
	machines     map[string]*fakeMachine
	stateServers voyeur.Value // of *state.StateServerInfo
	session      *fakeMongoSession
}

var (
	_ stateInterface = (*fakeState)(nil)
	_ stateMachine   = (*fakeMachine)(nil)
	_ mongoSession   = (*fakeMongoSession)(nil)
)

func newFakeState() *fakeState {
	st := &fakeState{
		machines: make(map[string]*fakeMachine),
		session:  newFakeMongoSession(),
	}
	st.stateServers.Set(&state.StateServerInfo{})
	return st
}

func (st *fakeState) MongoSession() mongoSession {
	return st.session
}

func (st *fakeState) Machine(id string) (stateMachine, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if m := st.machines[id]; m != nil {
		return m, nil
	}
	return nil, errors.NotFoundf("machine %s", id)
}

func (st *fakeState) addMachine(id string, wantsVote bool) *fakeMachine {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.machines[id] != nil {
		panic(fmt.Errorf("id %q already used", id))
	}
	m := &fakeMachine{
		doc: machineDoc{
			id:        id,
			wantsVote: wantsVote,
		},
	}
	m.val.Set(m.doc)
	return m
}

func (st *fakeState) removeMachine(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.machines[id] == nil {
		panic(fmt.Errorf("removing non-existent machine %q", id))
	}
	delete(st.machines, id)
}

func (st *fakeState) setStateServers(info *state.StateServerInfo) {
	st.stateServers.Set(info)
}

func (st *fakeState) StateServerInfo() (*state.StateServerInfo, error) {
	return deepCopy(st.stateServers.Get()).(*state.StateServerInfo), nil
}

func (st *fakeState) WatchStateServerInfo() state.NotifyWatcher {
	return WatchValue(&st.stateServers)
}

type fakeMachine struct {
	mu  sync.Mutex
	val voyeur.Value // of machineDoc
	doc machineDoc
}

func (m *fakeMachine) Refresh() error {
	m.doc = m.val.Get().(machineDoc)
	return nil
}

func (m *fakeMachine) Id() string {
	return m.doc.id
}

func (m *fakeMachine) Watch() state.NotifyWatcher {
	return WatchValue(&m.val)
}

func (m *fakeMachine) WantsVote() bool {
	return m.doc.wantsVote
}

func (m *fakeMachine) HasVote() bool {
	return m.doc.hasVote
}

func (m *fakeMachine) StateHostPort() string {
	return m.doc.hostPort
}

// mutate atomically changes the machineDoc of
// the receiver by mutating it with the provided function.
func (m *fakeMachine) mutate(f func(*machineDoc)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc := m.val.Get().(machineDoc)
	f(&doc)
	m.val.Set(doc)
	f(&m.doc)
}

func (m *fakeMachine) setStateHostPort(hostPort string) {
	m.mutate(func(doc *machineDoc) {
		doc.hostPort = hostPort
	})
}

func (m *fakeMachine) SetHasVote(hasVote bool) error {
	m.mutate(func(doc *machineDoc) {
		doc.hasVote = hasVote
	})
	return nil
}

func (m *fakeMachine) setWantsVote(wantsVote bool) error {
	m.mutate(func(doc *machineDoc) {
		doc.wantsVote = wantsVote
	})
	return nil
}

type machineDoc struct {
	id        string
	wantsVote bool
	hasVote   bool
	hostPort  string
}

type fakeMongoSession struct {
	err     error
	members voyeur.Value // of []replicaset.Member
	status  voyeur.Value // of *replicaset.Status
}

func newFakeMongoSession() *fakeMongoSession {
	s := new(fakeMongoSession)
	s.members.Set([]replicaset.Member(nil))
	s.status.Set(&replicaset.Status{})
	return s
}

func (session *fakeMongoSession) CurrentMembers() ([]replicaset.Member, error) {
	return deepCopy(session.members.Get()).([]replicaset.Member), nil
}

func (session *fakeMongoSession) CurrentStatus() (*replicaset.Status, error) {
	return deepCopy(session.status.Get()).(*replicaset.Status), nil
}

func (session *fakeMongoSession) Set(members []replicaset.Member) error {
	if session.err != nil {
		return session.err
	}
	session.members.Set(deepCopy(members))
	return nil
}

// deepCopy makes a deep copy of any type by marshalling
// it as JSON, then unmarshalling it.
func deepCopy(x interface{}) interface{} {
	v := reflect.ValueOf(x)
	data, err := json.Marshal(x)
	if err != nil {
		panic(fmt.Errorf("cannot marshal %#v: %v", x, err))
	}
	newv := reflect.New(v.Type())
	if err := json.Unmarshal(data, newv.Interface()); err != nil {
		panic(fmt.Errorf("cannot unmarshal %q into %s", data, newv.Type()))
	}
	// sanity check
	newx := newv.Elem().Interface()
	if !reflect.DeepEqual(newx, x) {
		panic(fmt.Errorf("value not deep-copied correctly"))
	}
	return newx
}

// notifier implements a value that can be
// watched for changes. Only one
type notifier struct {
	tomb    tomb.Tomb
	w       *voyeur.Watcher
	changes chan struct{}
}

func WatchValue(val *voyeur.Value) state.NotifyWatcher {
	n := &notifier{
		w:       val.Watch(),
		changes: make(chan struct{}),
	}
	go n.loop()
	return n
}

func (n *notifier) loop() {
	defer n.tomb.Done()
	for n.w.Next() {
		select {
		case n.changes <- struct{}{}:
		case <-n.tomb.Dying():
		}
	}
}

// Changes returns a channel that sends a value when the value changes.
// The value itself can be retrieved by calling the value's Get method.
func (n *notifier) Changes() <-chan struct{} {
	return n.changes
}

// Kill stops the notifier but does not wait for it to finish.
func (n *notifier) Kill() {
	n.tomb.Kill(nil)
	n.w.Close()
}

func (n *notifier) Err() error {
	return n.tomb.Err()
}

// Wait waits for the notifier to finish. It always returns nil.
func (n *notifier) Wait() error {
	return n.tomb.Wait()
}

func (n *notifier) Stop() error {
	return worker.Stop(n)
}
