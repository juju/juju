// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"sync"
	"time"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
)

type stateInterface interface {
	Machine(id string) (stateMachine, error)
	WatchStateServerInfo() state.NotifyWatcher
	StateServerInfo() (*state.StateServerInfo, error)
	MongoSession() mongoSession
}

type stateMachine interface {
	Id() string
	Refresh() error
	Watch() state.NotifyWatcher
	WantsVote() bool
	HasVote() bool
	SetHasVote(hasVote bool) error
	StateHostPort() string
}

type mongoSession interface {
	CurrentStatus() (*replicaset.Status, error)
	CurrentMembers() ([]replicaset.Member, error)
	Set([]replicaset.Member) error
}

// notifyFunc holds a function that is sent
// to the main worker loop to fetch new information
// when something changes. It reports whether
// the information has actually changed (and by implication
// whether the replica set may need to be changed).
type notifyFunc func() (bool, error)

var (
	// If we fail to set the mongo replica set members,
	// we retry at the following interval until we succeed.
	retryInterval = 2 * time.Second

	// pollInterval holds the interval at which the replica set
	// members will be updated even in the absence of changes
	// to State. This enables us to make changes to members
	// that are triggered by changes to member status.
	//
	// 10 seconds is the default time interval used by
	// mongo to keep its replicas up to date.
	pollInterval = 10 * time.Second
)

// pgWorker holds all the mutable state that we are watching.
// The only goroutine that is allowed to modify this
// is worker.loop - other watchers modify the
// current state by calling worker.notify instead of
// modifying it directly.
type pgWorker struct {
	tomb tomb.Tomb

	// wg represents all the currently running goroutines.
	// The worker main loop waits for all of these to exit
	// before finishing.
	wg sync.WaitGroup

	// st represents the State. It is an interface for testing
	// purposes only.
	st stateInterface

	// When something changes that might might affect
	// the peer group membership, it sends a function
	// on notifyCh that is run inside the main worker
	// goroutine to mutate the state. It reports whether
	// the state has actually changed.
	notifyCh chan notifyFunc

	// machines holds the set of machines we are currently
	// watching (all the state server machines). Each one has an
	// associated goroutine that
	// watches attributes of that machine.
	machines map[string]*machine
}

// New returns a new worker that maintains the mongo replica set
// with respect to the given state.
func New(st *state.State) (worker.Worker, error) {
	cfg, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	return newWorker(&stateShim{
		State:     st,
		mongoPort: cfg.StatePort(),
	}), nil
}

func newWorker(st stateInterface) worker.Worker {
	w := &pgWorker{
		st:       st,
		notifyCh: make(chan notifyFunc),
		machines: make(map[string]*machine),
	}
	go func() {
		defer w.tomb.Done()
		if err := w.loop(); err != nil {
			logger.Errorf("peergrouper loop terminated: %v", err)
			w.tomb.Kill(err)
		}
		// Wait for the various goroutines to be killed.
		// N.B. we don't defer this call because
		// if we do and a bug causes a panic, Wait will deadlock
		// waiting for the unkilled goroutines to exit.
		w.wg.Wait()
	}()
	return w
}

func (w *pgWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *pgWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *pgWorker) loop() error {
	infow := w.watchStateServerInfo()
	defer infow.stop()

	retry := time.NewTimer(0)
	retry.Stop()
	for {
		select {
		case f := <-w.notifyCh:
			// Update our current view of the state of affairs.
			changed, err := f()
			if err != nil {
				return err
			}
			if !changed {
				break
			}
			// Try to update the replica set immediately.
			retry.Reset(0)
		case <-retry.C:
			if err := w.updateReplicaset(); err != nil {
				if _, isReplicaSetError := err.(*replicaSetError); !isReplicaSetError {
					return err
				}
				logger.Errorf("cannot set replicaset: %v", err)
				retry.Reset(retryInterval)
				break
			}

			// Update the replica set members occasionally
			// to keep them up to date with the current
			// replica set member statuses.
			retry.Reset(pollInterval)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// notify sends the given notification function to
// the worker main loop to be executed.
func (w *pgWorker) notify(f notifyFunc) bool {
	select {
	case w.notifyCh <- f:
		return true
	case <-w.tomb.Dying():
		return false
	}
}

// getPeerGroupInfo collates current session information about the
// mongo peer group with information from state machines.
func (w *pgWorker) peerGroupInfo() (*peerGroupInfo, error) {
	session := w.st.MongoSession()
	info := &peerGroupInfo{}
	var err error
	status, err := session.CurrentStatus()
	if err != nil {
		return nil, fmt.Errorf("cannot get replica set status: %v", err)
	}
	info.statuses = status.Members
	info.members, err = session.CurrentMembers()
	if err != nil {
		return nil, fmt.Errorf("cannot get replica set members: %v", err)
	}
	info.machines = w.machines
	return info, nil
}

// replicaSetError holds an error returned as a result
// of calling replicaset.Set. As this is expected to fail
// in the normal course of things, it needs special treatment.
type replicaSetError struct {
	error
}

// updateReplicaset sets the current replica set members, and applies the
// given voting status to machines in the state.
func (w *pgWorker) updateReplicaset() error {
	info, err := w.peerGroupInfo()
	if err != nil {
		return err
	}
	members, voting, err := desiredPeerGroup(info)
	if err != nil {
		return fmt.Errorf("cannot compute desired peer group: %v", err)
	}
	if members == nil {
		logger.Debugf("no change in desired peer group")
		return nil
	}
	logger.Debugf("desired peer group members: %#v", members)
	// We cannot change the HasVote flag of a machine in state at exactly
	// the same moment as changing its voting status in the replica set.
	//
	// Thus we need to be careful that a machine which is actually a voting
	// member is not seen to not have a vote, because otherwise
	// there is nothing to prevent the machine being removed.
	//
	// To avoid this happening, we make sure when we call SetReplicaSet,
	// that the voting status of machines is the union of both old
	// and new voting machines - that is the set of HasVote machines
	// is a superset of all the actual voting machines.
	//
	// Only after the call has taken place do we reset the voting status
	// of the machines that have lost their vote.
	//
	// If there's a crash, the voting status may not reflect the
	// actual voting status for a while, but when things come
	// back on line, it will be sorted out, as desiredReplicaSet
	// will return the actual voting status.

	var added, removed []*machine
	for m, hasVote := range voting {
		switch {
		case hasVote && !m.stm.HasVote():
			added = append(added, m)
		case !hasVote && m.stm.HasVote():
			removed = append(removed, m)
		}
	}
	if err := setHasVote(added, true); err != nil {
		return err
	}
	if err := w.st.MongoSession().Set(members); err != nil {
		// We've failed to set the replica set, so revert back
		// to the previous settings.
		if err1 := setHasVote(added, false); err1 != nil {
			logger.Errorf("cannot revert machine voting after failure to change replica set: %v", err1)
		}
		return &replicaSetError{err}
	}
	logger.Infof("successfully changed replica set to %#v", members)
	if err := setHasVote(removed, false); err != nil {
		return err
	}
	return nil
}

// start runs the given loop function until it returns.
// When it returns, the receiving pgWorker is killed with
// the returned error.
func (w *pgWorker) start(loop func() error) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if err := loop(); err != nil {
			w.tomb.Kill(err)
		}
	}()
}

// setHasVote sets the HasVote status of all the given
// machines to hasVote.
func setHasVote(ms []*machine, hasVote bool) error {

	for _, m := range ms {
		if err := m.stm.SetHasVote(hasVote); err != nil {
			return fmt.Errorf("cannot set voting status of %q to %v: %v", m.id, hasVote, err)
		}
	}
	return nil
}

// serverInfoWatcher watches the state server info and
// notifies the worker when it changes.
type serverInfoWatcher struct {
	worker  *pgWorker
	watcher state.NotifyWatcher
}

func (w *pgWorker) watchStateServerInfo() *serverInfoWatcher {
	infow := &serverInfoWatcher{
		worker:  w,
		watcher: w.st.WatchStateServerInfo(),
	}
	w.start(infow.loop)
	return infow
}

func (infow *serverInfoWatcher) loop() error {
	for {
		select {
		case _, ok := <-infow.watcher.Changes():
			if !ok {
				return infow.watcher.Err()
			}
			infow.worker.notify(infow.updateMachines)
		case <-infow.worker.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

func (infow *serverInfoWatcher) stop() {
	infow.watcher.Stop()
}

// updateMachines is a notifyFunc that updates the current
// machines when the state server info has changed.
func (infow *serverInfoWatcher) updateMachines() (bool, error) {
	info, err := infow.worker.st.StateServerInfo()
	if err != nil {
		return false, fmt.Errorf("cannot get state server info: %v", err)
	}
	changed := false
	// Stop machine goroutines that no longer correspond to state server
	// machines.
	for _, m := range infow.worker.machines {
		if !inStrings(m.id, info.MachineIds) {
			m.stop()
			delete(infow.worker.machines, m.id)
			changed = true
		}
	}
	// Start machines with no watcher
	for _, id := range info.MachineIds {
		if _, ok := infow.worker.machines[id]; ok {
			continue
		}
		logger.Debugf("found new machine %q", id)
		stm, err := infow.worker.st.Machine(id)
		if err != nil {
			if errors.IsNotFoundError(err) {
				// If the machine isn't found, it must have been
				// removed and will soon enough be removed
				// from the state server list. This will probably
				// never happen, but we'll code defensively anyway.
				logger.Warningf("machine %q from state server list not found", id)
				continue
			}
			return false, fmt.Errorf("cannot get machine %q: %v", id, err)
		}
		infow.worker.machines[id] = infow.worker.newMachine(stm)
		changed = true
	}
	return changed, nil
}

// machine represents a machine in State.
type machine struct {
	id        string
	wantsVote bool
	hostPort  string

	worker         *pgWorker
	stm            stateMachine
	machineWatcher state.NotifyWatcher
}

func (m *machine) String() string {
	return m.id
}

func (m *machine) GoString() string {
	return fmt.Sprintf("&peergrouper.machine{id: %q, wantsVote: %v, hostPort: %q}", m.id, m.wantsVote, m.hostPort)
}

func (w *pgWorker) newMachine(stm stateMachine) *machine {
	m := &machine{
		worker:         w,
		id:             stm.Id(),
		stm:            stm,
		hostPort:       stm.StateHostPort(),
		wantsVote:      stm.WantsVote(),
		machineWatcher: stm.Watch(),
	}
	w.start(m.loop)
	return m
}

func (m *machine) loop() error {
	for {
		select {
		case _, ok := <-m.machineWatcher.Changes():
			if !ok {
				return m.machineWatcher.Err()
			}
			m.worker.notify(m.refresh)
		case <-m.worker.tomb.Dying():
			return nil
		}
	}
}

func (m *machine) stop() {
	m.machineWatcher.Stop()
}

func (m *machine) refresh() (bool, error) {
	if err := m.stm.Refresh(); err != nil {
		if errors.IsNotFoundError(err) {
			// We want to be robust when the machine
			// state is out of date with respect to the
			// state server info, so if the machine
			// has been removed, just assume that
			// no change has happened - the machine
			// loop will be stopped very soon anyway.
			return false, nil
		}
		return false, err
	}
	changed := false
	if wantsVote := m.stm.WantsVote(); wantsVote != m.wantsVote {
		m.wantsVote = wantsVote
		changed = true
	}
	if hostPort := m.stm.StateHostPort(); hostPort != m.hostPort {
		m.hostPort = hostPort
		changed = true
	}
	return changed, nil
}

func inStrings(t string, ss []string) bool {
	for _, s := range ss {
		if s == t {
			return true
		}
	}
	return false
}
