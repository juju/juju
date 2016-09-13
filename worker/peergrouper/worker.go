// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.peergrouper")

type stateInterface interface {
	Machine(id string) (stateMachine, error)
	WatchControllerInfo() state.NotifyWatcher
	WatchControllerStatusChanges() state.StringsWatcher
	ControllerInfo() (*state.ControllerInfo, error)
	MongoSession() mongoSession
	Space(id string) (SpaceReader, error)
	SetOrGetMongoSpaceName(spaceName network.SpaceName) (network.SpaceName, error)
	SetMongoSpaceState(mongoSpaceState state.MongoSpaceStates) error
}

type stateMachine interface {
	Id() string
	InstanceId() (instance.Id, error)
	Status() (status.StatusInfo, error)
	Refresh() error
	Watch() state.NotifyWatcher
	WantsVote() bool
	HasVote() bool
	SetHasVote(hasVote bool) error
	APIHostPorts() []network.HostPort
	MongoHostPorts() []network.HostPort
}

type mongoSession interface {
	CurrentStatus() (*replicaset.Status, error)
	CurrentMembers() ([]replicaset.Member, error)
	Set([]replicaset.Member) error
}

type publisherInterface interface {
	// publish publishes information about the given controllers
	// to whomsoever it may concern. When it is called there
	// is no guarantee that any of the information has actually changed.
	publishAPIServers(apiServers [][]network.HostPort, instanceIds []instance.Id) error
}

var (
	// If we fail to set the mongo replica set members,
	// we start retrying with the following interval,
	// before exponentially backing off with each further
	// attempt.
	initialRetryInterval = 2 * time.Second

	// maxRetryInterval holds the maximum interval
	// between retry attempts.
	maxRetryInterval = 5 * time.Minute

	// pollInterval holds the interval at which the replica set
	// members will be updated even in the absence of changes
	// to State. This enables us to make changes to members
	// that are triggered by changes to member status.
	pollInterval = 1 * time.Minute
)

// pgWorker is a worker which watches the controller machines in state
// as well as the MongoDB replicaset configuration, adding and
// removing controller machines as they change or are added and
// removed.
type pgWorker struct {
	catacomb catacomb.Catacomb

	// st represents the State. It is an interface so we can swap
	// out the implementation during testing.
	st stateInterface

	// machineChanges receives events from the machineTrackers when
	// controller machines change in ways that are relevant to the
	// peergrouper.
	machineChanges chan struct{}

	// machineTrackers holds the workers which track the machines we
	// are currently watching (all the controller machines).
	machineTrackers map[string]*machineTracker

	// publisher holds the implementation of the API
	// address publisher.
	publisher publisherInterface

	providerSupportsSpaces bool
}

// New returns a new worker that maintains the mongo replica set
// with respect to the given state.
func New(st *state.State, supportsSpaces bool) (worker.Worker, error) {
	cfg, err := st.ControllerConfig()
	if err != nil {
		return nil, err
	}
	shim := &stateShim{
		State:     st,
		mongoPort: cfg.StatePort(),
		apiPort:   cfg.APIPort(),
	}
	return newWorker(shim, newPublisher(st), supportsSpaces)
}

func newWorker(st stateInterface, pub publisherInterface, supportsSpaces bool) (worker.Worker, error) {
	w := &pgWorker{
		st:                     st,
		machineChanges:         make(chan struct{}),
		machineTrackers:        make(map[string]*machineTracker),
		publisher:              pub,
		providerSupportsSpaces: supportsSpaces,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (w *pgWorker) Kill() {
	w.catacomb.Kill(nil)
}

func (w *pgWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *pgWorker) loop() error {
	controllerChanges, err := w.watchForControllerChanges()
	if err != nil {
		return errors.Trace(err)
	}

	var updateChan <-chan time.Time
	retryInterval := initialRetryInterval

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-controllerChanges:
			changed, err := w.updateControllerMachines()
			if err != nil {
				return errors.Trace(err)
			}
			if changed {
				// A controller machine was added or removed, update
				// the replica set immediately.
				// TODO(fwereade): 2016-03-17 lp:1558657
				updateChan = time.After(0)
			}

		case <-w.machineChanges:
			// One of the controller machines changed, update the
			// replica set immediately.
			// TODO(fwereade): 2016-03-17 lp:1558657
			updateChan = time.After(0)

		case <-updateChan:
			ok := true
			servers, instanceIds, err := w.apiPublishInfo()
			if err != nil {
				return fmt.Errorf("cannot get API server info: %v", err)
			}
			if err := w.publisher.publishAPIServers(servers, instanceIds); err != nil {
				logger.Errorf("cannot publish API server addresses: %v", err)
				ok = false
			}
			if err := w.updateReplicaset(); err != nil {
				if _, isReplicaSetError := err.(*replicaSetError); !isReplicaSetError {
					return err
				}
				logger.Errorf("cannot set replicaset: %v", err)
				ok = false
			}
			if ok {
				// Update the replica set members occasionally
				// to keep them up to date with the current
				// replica set member statuses.
				// TODO(fwereade): 2016-03-17 lp:1558657
				updateChan = time.After(pollInterval)
				retryInterval = initialRetryInterval
			} else {
				// TODO(fwereade): 2016-03-17 lp:1558657
				updateChan = time.After(retryInterval)
				retryInterval *= 2
				if retryInterval > maxRetryInterval {
					retryInterval = maxRetryInterval
				}
			}

		}
	}
}

// watchForControllerChanges starts two watchers pertaining to changes
// to the controllers, returning a channel which will receive events
// if either watcher fires.
func (w *pgWorker) watchForControllerChanges() (<-chan struct{}, error) {
	controllerInfoWatcher := w.st.WatchControllerInfo()
	if err := w.catacomb.Add(controllerInfoWatcher); err != nil {
		return nil, errors.Trace(err)
	}

	controllerStatusWatcher := w.st.WatchControllerStatusChanges()
	if err := w.catacomb.Add(controllerStatusWatcher); err != nil {
		return nil, errors.Trace(err)
	}

	out := make(chan struct{})
	go func() {
		for {
			select {
			case <-w.catacomb.Dying():
				return
			case <-controllerInfoWatcher.Changes():
				out <- struct{}{}
			case <-controllerStatusWatcher.Changes():
				out <- struct{}{}
			}
		}
	}()
	return out, nil
}

// updateControllerMachines updates the peergrouper's current list of
// controller machines, as well as starting and stopping trackers for
// them as they are added and removed.
func (w *pgWorker) updateControllerMachines() (bool, error) {
	info, err := w.st.ControllerInfo()
	if err != nil {
		return false, fmt.Errorf("cannot get controller info: %v", err)
	}

	logger.Debugf("controller machines in state: %#v", info.MachineIds)
	changed := false

	// Stop machine goroutines that no longer correspond to controller
	// machines.
	for _, m := range w.machineTrackers {
		if !inStrings(m.Id(), info.MachineIds) {
			worker.Stop(m)
			delete(w.machineTrackers, m.Id())
			changed = true
		}
	}

	// Start machines with no watcher
	for _, id := range info.MachineIds {
		if _, ok := w.machineTrackers[id]; ok {
			continue
		}
		logger.Debugf("found new machine %q", id)
		stm, err := w.st.Machine(id)
		if err != nil {
			if errors.IsNotFound(err) {
				// If the machine isn't found, it must have been
				// removed and will soon enough be removed
				// from the controller list. This will probably
				// never happen, but we'll code defensively anyway.
				logger.Warningf("machine %q from controller list not found", id)
				continue
			}
			return false, fmt.Errorf("cannot get machine %q: %v", id, err)
		}

		// Don't add the machine unless it is "Started"
		machineStatus, err := stm.Status()
		if err != nil {
			return false, errors.Annotatef(err, "cannot get status for machine %q", id)
		}
		if machineStatus.Status == status.Started {
			logger.Debugf("machine %q has started, adding it to peergrouper list", id)
			tracker, err := newMachineTracker(stm, w.machineChanges)
			if err != nil {
				return false, errors.Trace(err)
			}
			if err := w.catacomb.Add(tracker); err != nil {
				return false, errors.Trace(err)
			}
			w.machineTrackers[id] = tracker
			changed = true
		} else {
			logger.Debugf("machine %q not ready: %v", id, machineStatus.Status)
		}

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

func (w *pgWorker) apiPublishInfo() ([][]network.HostPort, []instance.Id, error) {
	servers := make([][]network.HostPort, 0, len(w.machineTrackers))
	instanceIds := make([]instance.Id, 0, len(w.machineTrackers))
	for _, m := range w.machineTrackers {
		if len(m.APIHostPorts()) == 0 {
			continue
		}
		instanceId, err := m.stm.InstanceId()
		if err != nil {
			return nil, nil, err
		}
		instanceIds = append(instanceIds, instanceId)
		servers = append(servers, m.APIHostPorts())

	}
	return servers, instanceIds, nil
}

// peerGroupInfo collates current session information about the
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
	info.machineTrackers = w.machineTrackers

	spaceName, err := w.getMongoSpace(mongoAddresses(info.machineTrackers))
	if err != nil {
		return nil, err
	}
	info.mongoSpace = spaceName

	return info, nil
}

func mongoAddresses(machines map[string]*machineTracker) [][]network.Address {
	addresses := make([][]network.Address, len(machines))
	i := 0
	for _, m := range machines {
		for _, hp := range m.MongoHostPorts() {
			addresses[i] = append(addresses[i], hp.Address)
		}
		i++
	}
	return addresses
}

// getMongoSpace updates info with the space that Mongo servers should exist in.
func (w *pgWorker) getMongoSpace(addrs [][]network.Address) (network.SpaceName, error) {
	unset := network.SpaceName("")

	stateInfo, err := w.st.ControllerInfo()
	if err != nil {
		return unset, errors.Annotate(err, "cannot get state server info")
	}

	switch stateInfo.MongoSpaceState {
	case state.MongoSpaceUnknown:
		if !w.providerSupportsSpaces {
			err := w.st.SetMongoSpaceState(state.MongoSpaceUnsupported)
			if err != nil {
				return unset, errors.Annotate(err, "cannot set Mongo space state")
			}
			return unset, nil
		}

		// We want to find a space that contains all Mongo servers so we can
		// use it to look up the IP address of each Mongo server to be used
		// to set up the peer group.
		spaceStats := generateSpaceStats(addrs)
		if spaceStats.LargestSpaceContainsAll == false {
			err := w.st.SetMongoSpaceState(state.MongoSpaceInvalid)
			if err != nil {
				return unset, errors.Annotate(err, "cannot set Mongo space state")
			}
			logger.Warningf("couldn't find a space containing all peer group machines")
			return unset, nil
		} else {
			spaceName, err := w.st.SetOrGetMongoSpaceName(spaceStats.LargestSpace)
			if err != nil {
				return unset, errors.Annotate(err, "error setting/getting Mongo space")
			}
			return spaceName, nil
		}

	case state.MongoSpaceValid:
		space, err := w.st.Space(stateInfo.MongoSpaceName)
		if err != nil {
			return unset, errors.Annotate(err, "looking up space")
		}
		return network.SpaceName(space.Name()), nil
	}

	return unset, nil
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
		return errors.Annotate(err, "cannot get peergrouper info")
	}
	members, voting, err := desiredPeerGroup(info)
	if err != nil {
		return fmt.Errorf("cannot compute desired peer group: %v", err)
	}
	if members != nil {
		logger.Debugf("desired peer group members: %#v", members)
	} else {
		logger.Debugf("no change in desired peer group (voting %#v)", voting)
	}

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
	//
	// Note that we potentially update the HasVote status of the machines even
	// if the members have not changed.
	var added, removed []*machineTracker
	for m, hasVote := range voting {
		switch {
		case hasVote && !m.stm.HasVote():
			added = append(added, m)
		case !hasVote && m.stm.HasVote():
			removed = append(removed, m)
		}
	}
	if err := setHasVote(added, true); err != nil {
		return errors.Annotate(err, "cannot set HasVote added")
	}
	if members != nil {
		if err := w.st.MongoSession().Set(members); err != nil {
			// We've failed to set the replica set, so revert back
			// to the previous settings.
			if err1 := setHasVote(added, false); err1 != nil {
				logger.Errorf("cannot revert machine voting after failure to change replica set: %v", err1)
			}
			return &replicaSetError{err}
		}
		logger.Infof("successfully changed replica set to %#v", members)
	}
	if err := setHasVote(removed, false); err != nil {
		return errors.Annotate(err, "cannot set HasVote removed")
	}
	return nil
}

// setHasVote sets the HasVote status of all the given
// machines to hasVote.
func setHasVote(ms []*machineTracker, hasVote bool) error {
	if len(ms) == 0 {
		return nil
	}
	logger.Infof("setting HasVote=%v on machines %v", hasVote, ms)
	for _, m := range ms {
		if err := m.stm.SetHasVote(hasVote); err != nil {
			return fmt.Errorf("cannot set voting status of %q to %v: %v", m.Id(), hasVote, err)
		}
	}
	return nil
}

// allSpaceStats holds a SpaceStats for both API and Mongo machines
type allSpaceStats struct {
	APIMachines   spaceStats
	MongoMachines spaceStats
}

// SpaceStats holds information useful when choosing which space to pick an
// address from.
type spaceStats struct {
	SpaceRefCount           map[network.SpaceName]int
	LargestSpace            network.SpaceName
	LargestSpaceSize        int
	LargestSpaceContainsAll bool
}

// generateSpaceStats takes a list of machine addresses and returns information
// about what spaces are referenced by those machines.
func generateSpaceStats(addresses [][]network.Address) spaceStats {
	var stats spaceStats
	stats.SpaceRefCount = make(map[network.SpaceName]int)

	for i := range addresses {
		for _, addr := range addresses[i] {
			v := stats.SpaceRefCount[addr.SpaceName]
			v++
			stats.SpaceRefCount[addr.SpaceName] = v

			if v > stats.LargestSpaceSize {
				stats.LargestSpace = addr.SpaceName
				stats.LargestSpaceSize = v
			}
		}
	}

	stats.LargestSpaceContainsAll = stats.LargestSpaceSize == len(addresses)

	return stats
}
