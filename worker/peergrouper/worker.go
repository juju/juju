package peergrouper

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
)

// notifyFunc holds a function that is sent
// to the main worker loop to fetch new information
// when something changes. It reports whether
// the information has actually changed (and by implication
// whether the replica set may need to be changed).
type notifyFunc func() (bool, error)

const retryInterval = 2 * time.Second

// worker holds all the mutable state that we are watching.
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

	st *state.State

	// When something changes that might might affect
	// the peer group membership, it sends a function
	// on notifyCh that is run inside the main worker
	// goroutine to mutate the state. It reports whether
	// the state has actually changed.
	notifyCh chan notifyFunc

	// mongoPort holds the mongo port - it is set at initialisation
	// time, and never changes subsequently.
	mongoPort int

	// machines holds the set of machines we are currently
	// watching (all the state server machines). Each one has an
	// associated goroutine that
	// watches attributes of that machine.
	machines map[string]*machine
}

// New returns a new worker that maintains the mongo replica set
// with respect to the given state.
func New(st *state.State) worker.Worker {
	w := &pgWorker{
		st:       st,
		notifyCh: make(chan notifyFunc),
		machines: make(map[string]*machine),
	}
	go func() {
		defer w.tomb.Done()
		defer w.wg.Wait()
		if err := w.loop(); err != nil {
			w.tomb.Kill(err)
		}
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

	cfg, err := w.st.EnvironConfig()
	if err != nil {
		return err
	}
	w.mongoPort = cfg.StatePort()

	timer := time.NewTimer(0)
	timer.Stop()

	var desiredVoting map[*machine]bool
	var desiredMembers []replicaset.Member
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
			info, err := w.peerGroupInfo()
			if err != nil {
				return err
			}
			members, voting, err := desiredPeerGroup(info)
			if err != nil {
				logger.Errorf("cannot compute desired peer group: %v", err)
				continue
			}
			if members == nil {
				timer.Stop()
				break
			}
			desiredMembers = members
			desiredVoting = voting
			// Try to change the replica set immediately.
			timer.Reset(0)
		case <-timer.C:
			if err := w.setReplicaSet(desiredMembers, desiredVoting); err != nil {
				if _, isReplicaSetError := err.(*replicaSetError); !isReplicaSetError {
					return err
				}
				logger.Errorf("cannot set replicaset: %v", err)
				timer.Reset(retryInterval)
			}
			logger.Infof("successfully changed replica set to %#v", desiredMembers)
		}
	}
}

// getPeerGroupInfo collates current session information about the
// mongo peer group with information from state machines.
func (w *pgWorker) peerGroupInfo() (*peerGroupInfo, error) {
	session := w.st.MongoSession()
	info := &peerGroupInfo{}
	var err error
	status, err := replicaset.CurrentStatus(session)
	if err != nil {
		return nil, fmt.Errorf("cannot get replica set status: %v", err)
	}
	info.statuses = status.Members
	info.members, err = replicaset.CurrentMembers(session)
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

// setReplicaSet sets the current replica set members, and applies the
// given voting status to machines in the state.
func (w *pgWorker) setReplicaSet(members []replicaset.Member, voting map[*machine]bool) error {
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
	if err := replicaset.Set(w.st.MongoSession(), members); err != nil {
		// We've failed to set the replica set, so revert back
		// to the previous settings.
		if err1 := setHasVote(added, false); err1 != nil {
			log.Errorf("cannot revert machine voting after failure to change replica set")
		}
		return &replicaSetError{err}
	}
	if err := setHasVote(removed, false); err != nil {
		return err
	}
	return nil
}

// setHasVote sets the HasVote status of all the given
// machines to hasVote.
func setHasVote(ms []*machine, hasVote bool) error {
	for _, m := range ms {
		if err := m.stm.SetHasVote(hasVote); err != nil {
			return err
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
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if err := infow.loop(); err != nil {
			w.tomb.Kill(err)
		}
	}()
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
		return false, err
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
		stm, err := infow.worker.st.Machine(id)
		if err != nil {
			if errors.IsNotFoundError(err) {
				// If the machine isn't found, it must have been
				// removed and will soon enough be removed
				// from the state server list. This will probably
				// never happen, but we'll code defensively anyway.
				continue
			}
		}
		infow.worker.machines[id] = infow.worker.newMachine(stm)
		changed = true
	}
	return changed, nil
}

// notifyError sends a notification that returns the given error.
func (w *pgWorker) notifyError(err error) {
	w.notify(func() (bool, error) {
		return false, err
	})
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

// machine represents a machine in State.
type machine struct {
	id        string
	wantsVote bool
	hostPort  string

	worker         *pgWorker
	stm            *state.Machine
	machineWatcher state.NotifyWatcher
}

func (w *pgWorker) newMachine(stm *state.Machine) *machine {
	m := &machine{
		worker:         w,
		id:             stm.Id(),
		stm:            stm,
		machineWatcher: stm.Watch(),
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if err := m.loop(); err != nil {
			w.notifyError(err)
		}
	}()
	return m
}

func (m *machine) loop() error {
	defer m.worker.wg.Done()
	for {
		select {
		case _, ok := <-m.machineWatcher.Changes():
			if !ok {
				return m.machineWatcher.Err()
			}
			m.worker.notify(m.refresh)
		case <-m.worker.tomb.Dying():
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
	hostPort := instance.SelectInternalAddress(m.stm.Addresses(), false)
	if hostPort != "" {
		hostPort = joinHostPort(hostPort, m.worker.mongoPort)
	}
	if hostPort != m.hostPort {
		m.hostPort = hostPort
		changed = true
	}
	return changed, nil
}

func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func inStrings(t string, ss []string) bool {
	for _, s := range ss {
		if s == t {
			return true
		}
	}
	return false
}
