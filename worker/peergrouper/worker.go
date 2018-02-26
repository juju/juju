// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/network"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.peergrouper")

type State interface {
	ControllerConfig() (controller.Config, error)
	ControllerInfo() (*state.ControllerInfo, error)
	Machine(id string) (Machine, error)
	Space(id string) (Space, error)
	WatchControllerInfo() state.NotifyWatcher
	WatchControllerStatusChanges() state.StringsWatcher
}

type Space interface {
	Name() string
}

type Machine interface {
	Id() string
	Status() (status.StatusInfo, error)
	Refresh() error
	Watch() state.NotifyWatcher
	WantsVote() bool
	HasVote() bool
	SetHasVote(hasVote bool) error
	Addresses() []network.Address
}

type MongoSession interface {
	CurrentStatus() (*replicaset.Status, error)
	CurrentMembers() ([]replicaset.Member, error)
	Set([]replicaset.Member) error
}

type APIHostPortsSetter interface {
	SetAPIHostPorts([][]network.HostPort) error
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

// Hub defines the only method of the apiserver centralhub that
// the peer grouper uses.
type Hub interface {
	Publish(topic string, data interface{}) (<-chan struct{}, error)
}

// pgWorker is a worker which watches the controller machines in state
// as well as the MongoDB replicaset configuration, adding and
// removing controller machines as they change or are added and
// removed.
type pgWorker struct {
	catacomb catacomb.Catacomb

	config Config

	// machineChanges receives events from the machineTrackers when
	// controller machines change in ways that are relevant to the
	// peergrouper.
	machineChanges chan struct{}

	// machineTrackers holds the workers which track the machines we
	// are currently watching (all the controller machines).
	machineTrackers map[string]*machineTracker
}

// Config holds the configuration for a peergrouper worker.
type Config struct {
	State              State
	APIHostPortsSetter APIHostPortsSetter
	MongoSession       MongoSession
	Clock              clock.Clock
	SupportsSpaces     bool
	MongoPort          int
	APIPort            int

	// Hub is the central hub of the apiserver,
	// and is used to publish the details of the
	// API servers.
	Hub Hub
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.State == nil {
		return errors.NotValidf("nil State")
	}
	if config.APIHostPortsSetter == nil {
		return errors.NotValidf("nil APIHostPortsSetter")
	}
	if config.MongoSession == nil {
		return errors.NotValidf("nil MongoSession")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.MongoPort <= 0 {
		return errors.NotValidf("non-positive MongoPort")
	}
	if config.APIPort <= 0 {
		return errors.NotValidf("non-positive APIPort")
	}
	return nil
}

// New returns a new worker that maintains the mongo replica set
// with respect to the given state.
func New(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &pgWorker{
		config:          config,
		machineChanges:  make(chan struct{}),
		machineTrackers: make(map[string]*machineTracker),
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

// Kill is part of the worker.Worker interface.
func (w *pgWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
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
		logger.Tracef("waiting...")
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-controllerChanges:
			logger.Tracef("<-controllerChanges")
			changed, err := w.updateControllerMachines()
			if err != nil {
				return errors.Trace(err)
			}
			if !changed {
				continue
			}
			// A controller machine was added or removed.
			logger.Tracef("controller added or removed, update replica now")
		case <-w.machineChanges:
			logger.Tracef("<-w.machineChanges")
			// One of the controller machines changed.
		case <-updateChan:
			logger.Tracef("<-updateChan")
			// Scheduled update.
		}

		servers := w.apiServerHostPorts()
		apiHostPorts := make([][]network.HostPort, 0, len(servers))
		for _, serverHostPorts := range servers {
			apiHostPorts = append(apiHostPorts, serverHostPorts)
		}

		var failed bool
		if err := w.config.APIHostPortsSetter.SetAPIHostPorts(apiHostPorts); err != nil {
			logger.Errorf("cannot publish API server addresses: %v", err)
			failed = true
		}

		members, err := w.updateReplicaSet()
		if err != nil {
			if _, isReplicaSetError := err.(*replicaSetError); !isReplicaSetError {
				return err
			}
			logger.Errorf("cannot set replicaset: %v", err)
			failed = true
		}
		w.publishAPIServerDetails(servers, members)

		if failed {
			updateChan = w.config.Clock.After(retryInterval)
			retryInterval = scaleRetry(retryInterval)
		} else {
			// Update the replica set members occasionally
			// to keep them up to date with the current
			// replica set member statuses.
			updateChan = w.config.Clock.After(pollInterval)
			retryInterval = initialRetryInterval
		}
	}
}

func scaleRetry(value time.Duration) time.Duration {
	value *= 2
	if value > maxRetryInterval {
		value = maxRetryInterval
	}
	return value
}

// watchForControllerChanges starts two watchers pertaining to changes
// to the controllers, returning a channel which will receive events
// if either watcher fires.
func (w *pgWorker) watchForControllerChanges() (<-chan struct{}, error) {
	controllerInfoWatcher := w.config.State.WatchControllerInfo()
	if err := w.catacomb.Add(controllerInfoWatcher); err != nil {
		return nil, errors.Trace(err)
	}

	controllerStatusWatcher := w.config.State.WatchControllerStatusChanges()
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
	info, err := w.config.State.ControllerInfo()
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
		stm, err := w.config.State.Machine(id)
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

// apiServerHostPorts returns the host-ports for each apiserver machine.
func (w *pgWorker) apiServerHostPorts() map[string][]network.HostPort {
	servers := make(map[string][]network.HostPort)
	for _, m := range w.machineTrackers {
		hostPorts := network.AddressesWithPort(m.Addresses(), w.config.APIPort)
		if len(hostPorts) == 0 {
			continue
		}
		servers[m.Id()] = hostPorts
	}
	return servers
}

func (w *pgWorker) publishAPIServerDetails(
	servers map[string][]network.HostPort,
	members map[string]replicaset.Member,
) {
	details := apiserver.Details{
		Servers:   make(map[string]apiserver.APIServer),
		LocalOnly: true,
	}
	for id, hostPorts := range servers {
		var internalAddress string
		mongoAddress, _, err := net.SplitHostPort(members[id].Address)
		if err == nil {
			internalAddress = net.JoinHostPort(
				mongoAddress,
				strconv.Itoa(w.config.APIPort),
			)
		}
		server := apiserver.APIServer{
			ID:              id,
			InternalAddress: internalAddress,
		}
		for _, hp := range network.FilterUnusableHostPorts(hostPorts) {
			server.Addresses = append(server.Addresses, hp.String())
		}
		details.Servers[server.ID] = server
	}
	w.config.Hub.Publish(apiserver.DetailsTopic, details)
}

// replicaSetError holds an error returned as a result
// of calling replicaset.Set. As this is expected to fail
// in the normal course of things, it needs special treatment.
type replicaSetError struct {
	error
}

func prettyReplicaSetMembers(members []replicaset.Member) string {
	var result []string
	for _, member := range members {
		vote := true
		if member.Votes != nil && *member.Votes == 0 {
			vote = false
		}
		result = append(result, fmt.Sprintf("    Id: %d, Tags: %v, Vote: %v", member.Id, member.Tags, vote))
	}
	return strings.Join(result, "\n")
}

// updateReplicaSet sets the current replica set members, and applies the
// given voting status to machines in the state. A mapping of machine ID
// to replicaset.Member structures is returned.
func (w *pgWorker) updateReplicaSet() (map[string]replicaset.Member, error) {
	info, err := w.peerGroupInfo()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get peergrouper info")
	}
	members, voting, err := desiredPeerGroup(info)
	if err != nil {
		return nil, errors.Annotate(err, "cannot compute desired peer group")
	}
	if logger.IsDebugEnabled() {
		if members != nil {
			logger.Debugf("desired peer group members: \n%s", prettyReplicaSetMembers(members))
		} else {
			var output []string
			for m, v := range voting {
				output = append(output, fmt.Sprintf("  %s: %v", m.id, v))
			}
			logger.Debugf("no change in desired peer group, voting: \n%s", strings.Join(output, "\n"))
		}
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
		return nil, errors.Annotate(err, "cannot set HasVote added")
	}
	if members != nil {
		if err := w.config.MongoSession.Set(members); err != nil {
			// We've failed to set the replica set, so revert back
			// to the previous settings.
			if err1 := setHasVote(added, false); err1 != nil {
				logger.Errorf("cannot revert machine voting after failure to change replica set: %v", err1)
			}
			return nil, &replicaSetError{err}
		}
		logger.Infof("successfully updated replica set")
	}
	if err := setHasVote(removed, false); err != nil {
		return nil, errors.Annotate(err, "cannot set HasVote removed")
	}

	mm, _, _ := info.membersMap()
	machineMembers := make(map[string]replicaset.Member)
	for m, member := range mm {
		machineMembers[m.Id()] = *member
	}
	return machineMembers, nil
}

// peerGroupInfo collates current session information about the
// mongo peer group with information from state machines.
func (w *pgWorker) peerGroupInfo() (*peerGroupInfo, error) {
	info := &peerGroupInfo{
		mongoPort: w.config.MongoPort,
	}

	sts, err := w.config.MongoSession.CurrentStatus()
	if err != nil {
		return nil, fmt.Errorf("cannot get replica set status: %v", err)
	}
	info.statuses = sts.Members

	info.members, err = w.config.MongoSession.CurrentMembers()
	if err != nil {
		return nil, fmt.Errorf("cannot get replica set members: %v", err)
	}
	info.machineTrackers = w.machineTrackers

	spaceName, err := w.getHASpaceFromConfig()
	if err != nil {
		return nil, err
	}
	info.haSpace = spaceName

	return info, nil
}

// getHASpaceFromConfig returns a SpaceName from the controller config for
// HA space. If unset, the empty space ("") will be returned.
func (w *pgWorker) getHASpaceFromConfig() (network.SpaceName, error) {
	config, err := w.config.State.ControllerConfig()
	if err != nil {
		return network.SpaceName(""), err
	}
	return network.SpaceName(config.JujuHASpace()), nil
}

// setHasVote sets the HasVote status of all the given machines to hasVote.
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
