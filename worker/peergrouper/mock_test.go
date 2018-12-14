// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path"
	"reflect"
	"strconv"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/replicaset"
	"github.com/juju/utils/voyeur"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// This file holds helper functions for mocking pieces of State and replicaset
// that we don't want to directly depend on in unit tests.

type fakeState struct {
	mu               sync.Mutex
	errors           errorPatterns
	machines         map[string]*fakeMachine
	controllers      voyeur.Value // of *state.ControllerInfo
	statuses         voyeur.Value // of statuses collection
	controllerConfig voyeur.Value // of controller.Config
	session          *fakeMongoSession
	checkMu          sync.Mutex
	check            func(st *fakeState) error
}

var (
	_ State        = (*fakeState)(nil)
	_ Machine      = (*fakeMachine)(nil)
	_ MongoSession = (*fakeMongoSession)(nil)
)

type errorPatterns struct {
	patterns []errorPattern
}

type errorPattern struct {
	pattern string
	errFunc func() error
}

// setErrorFor causes the given error to be returned
// from any mock call that matches the given
// string, which may contain wildcards as
// in path.Match.
//
// The standard form for errors is:
//    Type.Function <arg>...
// See individual functions for details.
func (e *errorPatterns) setErrorFor(what string, err error) {
	e.setErrorFuncFor(what, func() error {
		return err
	})
}

// setErrorFuncFor causes the given function
// to be invoked to return the error for the
// given pattern.
func (e *errorPatterns) setErrorFuncFor(what string, errFunc func() error) {
	e.patterns = append(e.patterns, errorPattern{
		pattern: what,
		errFunc: errFunc,
	})
}

// errorFor concatenates the call name
// with all the args, space separated,
// and returns any error registered with
// setErrorFor that matches the resulting string.
func (e *errorPatterns) errorFor(name string, args ...interface{}) error {
	s := name
	for _, arg := range args {
		s += " " + fmt.Sprint(arg)
	}
	f := func() error { return nil }
	for _, pattern := range e.patterns {
		if ok, _ := path.Match(pattern.pattern, s); ok {
			f = pattern.errFunc
			break
		}
	}
	err := f()
	if err != nil {
		logger.Errorf("errorFor %q -> %v", s, err)
	}
	return err
}

func (e *errorPatterns) resetErrors() {
	e.patterns = e.patterns[:0]
}

func NewFakeState() *fakeState {
	st := &fakeState{
		machines: make(map[string]*fakeMachine),
	}
	st.session = newFakeMongoSession(st, &st.errors)
	st.controllerConfig.Set(controller.Config{})
	return st
}

func (st *fakeState) getCheck() func(st *fakeState) error {
	st.checkMu.Lock()
	check := st.check
	st.checkMu.Unlock()
	return check
}

func (st *fakeState) setCheck(check func(st *fakeState) error) {
	st.checkMu.Lock()
	st.check = check
	st.checkMu.Unlock()
}

func (st *fakeState) checkInvariants() {
	check := st.getCheck()
	if check == nil {
		return
	}
	if err := check(st); err != nil {
		// Force a panic, otherwise we can deadlock
		// when called from within the worker.
		go panic(err)
		select {}
	}
}

// checkInvariants checks that all the expected invariants
// in the state hold true. Currently we check that:
// - total number of votes is odd.
// - member voting status implies that machine has vote.
func checkInvariants(st *fakeState) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	members := st.session.members.Get().([]replicaset.Member)
	voteCount := 0
	for _, m := range members {
		votes := 1
		if m.Votes != nil {
			votes = *m.Votes
		}
		voteCount += votes
		if id, ok := m.Tags[jujuMachineKey]; ok {
			if votes > 0 {
				m := st.machines[id]
				if m == nil {
					return fmt.Errorf("voting member with machine id %q has no associated Machine", id)
				}

				if !m.doc().hasVote {
					return fmt.Errorf("machine %q should be marked as having the vote, but does not", id)
				}
			}
		}
	}
	if voteCount%2 != 1 {
		return fmt.Errorf("total vote count is not odd (got %d)", voteCount)
	}
	return nil
}

type invariantChecker interface {
	checkInvariants()
}

// machine is similar to Machine except that
// it bypasses the error mocking machinery.
// It returns nil if there is no machine with the
// given id.
func (st *fakeState) machine(id string) *fakeMachine {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.machines[id]
}

func (st *fakeState) Machine(id string) (Machine, error) {
	if err := st.errors.errorFor("State.Machine", id); err != nil {
		return nil, err
	}
	if m := st.machine(id); m != nil {
		return m, nil
	}
	return nil, errors.NotFoundf("machine %s", id)
}

func (st *fakeState) addMachine(id string, wantsVote bool) *fakeMachine {
	st.mu.Lock()
	defer st.mu.Unlock()
	logger.Infof("fakeState.addMachine %q", id)
	if st.machines[id] != nil {
		panic(fmt.Errorf("id %q already used", id))
	}
	doc := machineDoc{
		id:         id,
		wantsVote:  wantsVote,
		statusInfo: status.StatusInfo{Status: status.Started},
		life:       state.Alive,
	}
	m := &fakeMachine{
		errors:  &st.errors,
		checker: st,
	}
	st.machines[id] = m
	m.val.Set(doc)
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

func (st *fakeState) setControllers(ids ...string) {
	st.controllers.Set(&state.ControllerInfo{
		MachineIds: ids,
	})
}

func (st *fakeState) ControllerInfo() (*state.ControllerInfo, error) {
	if err := st.errors.errorFor("State.ControllerInfo"); err != nil {
		return nil, err
	}
	return deepCopy(st.controllers.Get()).(*state.ControllerInfo), nil
}

func (st *fakeState) WatchControllerInfo() state.NotifyWatcher {
	return WatchValue(&st.controllers)
}

func (st *fakeState) WatchControllerStatusChanges() state.StringsWatcher {
	return WatchStrings(&st.statuses)
}

func (st *fakeState) WatchControllerConfig() state.NotifyWatcher {
	return WatchValue(&st.controllerConfig)
}

func (st *fakeState) ModelConfig() (*config.Config, error) {
	attrs := coretesting.FakeConfig()
	cfg, err := config.New(config.NoDefaults, attrs)
	return cfg, err
}

func (st *fakeState) ControllerConfig() (controller.Config, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if err := st.errors.errorFor("State.ControllerConfig"); err != nil {
		return nil, err
	}
	return deepCopy(st.controllerConfig.Get()).(controller.Config), nil
}

func (st *fakeState) RemoveControllerMachine(m Machine) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	controllerInfo := st.controllers.Get().(*state.ControllerInfo)
	machineIds := controllerInfo.MachineIds
	var newMachineIds []string
	machineId := m.Id()
	for _, id := range machineIds {
		if id == machineId {
			continue
		}
		newMachineIds = append(newMachineIds, id)
	}
	st.setControllers(newMachineIds...)
	return nil
}

func (st *fakeState) setHASpace(spaceName string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	cfg := st.controllerConfig.Get().(controller.Config)
	cfg[controller.JujuHASpace] = spaceName
	st.controllerConfig.Set(cfg)
}

type fakeMachine struct {
	mu      sync.Mutex
	errors  *errorPatterns
	val     voyeur.Value // of machineDoc
	checker invariantChecker
}

type machineDoc struct {
	id         string
	wantsVote  bool
	hasVote    bool
	instanceId instance.Id
	addresses  []network.Address
	statusInfo status.StatusInfo
	life       state.Life
}

func (m *fakeMachine) doc() machineDoc {
	return m.val.Get().(machineDoc)
}

func (m *fakeMachine) Refresh() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc := m.doc()
	if err := m.errors.errorFor("Machine.Refresh", doc.id); err != nil {
		return err
	}
	return nil
}

func (m *fakeMachine) GoString() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fmt.Sprintf("&fakeMachine{%#v}", m.doc())
}

func (m *fakeMachine) Id() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doc().id
}

func (m *fakeMachine) Life() state.Life {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doc().life
}

func (m *fakeMachine) Watch() state.NotifyWatcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	return WatchValue(&m.val)
}

func (m *fakeMachine) WantsVote() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doc().wantsVote
}

func (m *fakeMachine) HasVote() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doc().hasVote
}

func (m *fakeMachine) Addresses() []network.Address {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doc().addresses
}

func (m *fakeMachine) Status() (status.StatusInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doc().statusInfo, nil
}

func (m *fakeMachine) SetStatus(sInfo status.StatusInfo) error {
	m.mutate(func(doc *machineDoc) {
		doc.statusInfo = sInfo
	})
	return nil
}

// mutate atomically changes the machineDoc of
// the receiver by mutating it with the provided function.
func (m *fakeMachine) mutate(f func(*machineDoc)) {
	m.mu.Lock()
	doc := m.doc()
	f(&doc)
	m.val.Set(doc)
	m.checker.checkInvariants()
	m.mu.Unlock()
}

func (m *fakeMachine) setAddresses(addrs ...network.Address) {
	m.mutate(func(doc *machineDoc) {
		doc.addresses = addrs
	})
}

// SetHasVote implements Machine.SetHasVote.
func (m *fakeMachine) SetHasVote(hasVote bool) error {
	doc := m.doc()
	if err := m.errors.errorFor("Machine.SetHasVote", doc.id, hasVote); err != nil {
		return err
	}
	m.mutate(func(doc *machineDoc) {
		doc.hasVote = hasVote
	})
	return nil
}

func (m *fakeMachine) setWantsVote(wantsVote bool) {
	m.mutate(func(doc *machineDoc) {
		doc.wantsVote = wantsVote
	})
}

func (m *fakeMachine) advanceLifecycle(life state.Life, wantsVote bool) {
	m.mutate(func(doc *machineDoc) {
		doc.life = life
		doc.wantsVote = wantsVote
	})
}

type fakeMongoSession struct {
	// If InstantlyReady is true, replica status of
	// all members will be instantly reported as ready.
	InstantlyReady bool

	errors  *errorPatterns
	checker invariantChecker
	members voyeur.Value // of []replicaset.Member
	status  voyeur.Value // of *replicaset.Status
}

// newFakeMongoSession returns a mock implementation of mongoSession.
func newFakeMongoSession(checker invariantChecker, errors *errorPatterns) *fakeMongoSession {
	s := new(fakeMongoSession)
	s.checker = checker
	s.errors = errors
	return s
}

// CurrentMembers implements mongoSession.CurrentMembers.
func (session *fakeMongoSession) CurrentMembers() ([]replicaset.Member, error) {
	if err := session.errors.errorFor("Session.CurrentMembers"); err != nil {
		return nil, err
	}
	return deepCopy(session.members.Get()).([]replicaset.Member), nil
}

// CurrentStatus implements mongoSession.CurrentStatus.
func (session *fakeMongoSession) CurrentStatus() (*replicaset.Status, error) {
	if err := session.errors.errorFor("Session.CurrentStatus"); err != nil {
		return nil, err
	}
	return deepCopy(session.status.Get()).(*replicaset.Status), nil
}

// setStatus sets the status of the current members of the session.
func (session *fakeMongoSession) setStatus(members []replicaset.MemberStatus) {
	session.status.Set(deepCopy(&replicaset.Status{
		Members: members,
	}))
}

// Set implements mongoSession.Set
func (session *fakeMongoSession) Set(members []replicaset.Member) error {
	if err := session.errors.errorFor("Session.Set"); err != nil {
		logger.Infof("NOT setting replicaset members to \n%s", prettyReplicaSetMembersSlice(members))
		return err
	}
	logger.Infof("setting replicaset members to \n%s", prettyReplicaSetMembersSlice(members))
	session.members.Set(deepCopy(members))
	if session.InstantlyReady {
		statuses := make([]replicaset.MemberStatus, len(members))
		for i, m := range members {
			statuses[i] = replicaset.MemberStatus{
				Id:      m.Id,
				Address: m.Address,
				Healthy: true,
				State:   replicaset.SecondaryState,
			}
			if i == 0 {
				statuses[i].State = replicaset.PrimaryState
			}
		}
		session.setStatus(statuses)
	}
	session.checker.checkInvariants()
	return nil
}

func (session *fakeMongoSession) StepDownPrimary() error {
	if err := session.errors.errorFor("Session.StepDownPrimary"); err != nil {
		logger.Debugf("StepDownPrimary error: %v", err)
		return err
	}
	logger.Debugf("StepDownPrimary")
	status := session.status.Get().(*replicaset.Status)
	members := session.members.Get().([]replicaset.Member)
	membersById := make(map[int]replicaset.Member, len(members))
	for _, member := range members {
		membersById[member.Id] = member
	}
	// We use a simple algorithm, find the primary, and all the secondaries that don't have 0 priority. And then pick a
	// random secondary and swap their states
	primaryIndex := -1
	secondaryIndexes := []int{}
	var info []string
	for i, statusMember := range status.Members {
		if statusMember.State == replicaset.PrimaryState {
			primaryIndex = i
			info = append(info, fmt.Sprintf("%d: current primary", statusMember.Id))
		} else if statusMember.State == replicaset.SecondaryState {
			confMember := membersById[statusMember.Id]
			if confMember.Priority == nil || *confMember.Priority > 0 {
				secondaryIndexes = append(secondaryIndexes, i)
				info = append(info, fmt.Sprintf("%d: eligible secondary", statusMember.Id))
			} else {
				info = append(info, fmt.Sprintf("%d: ineligible secondary", statusMember.Id))
			}
		}
	}
	if primaryIndex == -1 {
		return errors.Errorf("no primary to step down, broken config?")
	}
	if len(secondaryIndexes) < 1 {
		return errors.Errorf("no secondaries to switch to")
	}
	secondaryIndex := secondaryIndexes[rand.Intn(len(secondaryIndexes))]
	status.Members[primaryIndex].State = replicaset.SecondaryState
	status.Members[secondaryIndex].State = replicaset.PrimaryState
	logger.Debugf("StepDownPrimary nominated %d to be the new primary from: %v",
		status.Members[secondaryIndex].Id, info)
	session.setStatus(status.Members)
	return nil
}

func (session *fakeMongoSession) Refresh() {
	// If this was a testing.Stub we would track that Refresh was called.
}

// prettyReplicaSetMembersSlice wraps prettyReplicaSetMembers for testing
// purposes only.
func prettyReplicaSetMembersSlice(members []replicaset.Member) string {
	vrm := make(map[string]*replicaset.Member, len(members))
	for i := range members {
		m := members[i]
		vrm[strconv.Itoa(m.Id)] = &m
	}
	return prettyReplicaSetMembers(vrm)
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

type notifier struct {
	tomb    tomb.Tomb
	w       *voyeur.Watcher
	changes chan struct{}
}

func (n *notifier) loop() {
	for n.w.Next() {
		select {
		case n.changes <- struct{}{}:
		case <-n.tomb.Dying():
		}
	}
}

// WatchValue returns a NotifyWatcher that triggers
// when the given value changes. Its Wait and Err methods
// never return a non-nil error.
func WatchValue(val *voyeur.Value) state.NotifyWatcher {
	n := &notifier{
		w:       val.Watch(),
		changes: make(chan struct{}),
	}
	n.tomb.Go(func() error {
		n.loop()
		return nil
	})
	return n
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

type stringsNotifier struct {
	tomb    tomb.Tomb
	w       *voyeur.Watcher
	changes chan []string
}

// WatchStrings returns a StringsWatcher that triggers
// when the given value changes. Its Wait and Err methods
// never return a non-nil error.
func WatchStrings(val *voyeur.Value) state.StringsWatcher {
	n := &stringsNotifier{
		w:       val.Watch(),
		changes: make(chan []string),
	}
	n.tomb.Go(func() error {
		n.loop()
		return nil
	})
	return n
}

func (n *stringsNotifier) loop() {
	for n.w.Next() {
		select {
		case n.changes <- []string{}:
		case <-n.tomb.Dying():
		}
	}
}

// Changes returns a channel that sends a value when the value changes.
// The value itself can be retrieved by calling the value's Get method.
func (n *stringsNotifier) Changes() <-chan []string {
	return n.changes
}

// Kill stops the notifier but does not wait for it to finish.
func (n *stringsNotifier) Kill() {
	n.tomb.Kill(nil)
	n.w.Close()
}

func (n *stringsNotifier) Err() error {
	return n.tomb.Err()
}

// Wait waits for the notifier to finish. It always returns nil.
func (n *stringsNotifier) Wait() error {
	return n.tomb.Wait()
}

func (n *stringsNotifier) Stop() error {
	return worker.Stop(n)
}
