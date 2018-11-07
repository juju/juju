// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
	"github.com/juju/utils/clock"
	"github.com/kr/pretty"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/network"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

var logger = loggo.GetLogger("juju.worker.peergrouper")

type State interface {
	RemoveControllerMachine(m Machine) error
	ControllerConfig() (controller.Config, error)
	ControllerInfo() (*state.ControllerInfo, error)
	Machine(id string) (Machine, error)
	WatchControllerInfo() state.NotifyWatcher
	WatchControllerStatusChanges() state.StringsWatcher
	WatchControllerConfig() state.NotifyWatcher
}

type Space interface {
	Name() string
}

type Machine interface {
	Id() string
	Life() state.Life
	Status() (status.StatusInfo, error)
	SetStatus(status.StatusInfo) error
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
	StepDownPrimary() error
	Refresh()
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

// Hub defines the methods of the apiserver centralhub that the peer
// grouper uses.
type Hub interface {
	Subscribe(topic string, handler interface{}) (func(), error)
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

	// detailsRequests is used to feed details requests from the hub into the main loop.
	detailsRequests chan string

	// serverDetails holds the last server information broadcast via pub/sub.
	// It is used to detect changes since the last publish.
	serverDetails apiserver.Details
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
	ControllerAPIPort  int

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
	// TODO Juju 3.0: make ControllerAPIPort required.
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
		detailsRequests: make(chan string),
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

	configChanges, err := w.watchForConfigChanges()
	if err != nil {
		return errors.Trace(err)
	}

	unsubscribe, err := w.config.Hub.Subscribe(apiserver.DetailsRequestTopic, w.apiserverDetailsRequested)
	if err != nil {
		return errors.Trace(err)
	}
	defer unsubscribe()

	var updateChan <-chan time.Time
	retryInterval := initialRetryInterval

	for {
		logger.Tracef("waiting...")
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-controllerChanges:
			// A controller machine was added or removed.
			logger.Tracef("<-controllerChanges")
			changed, err := w.updateControllerMachines()
			if err != nil {
				return errors.Trace(err)
			}
			if !changed {
				continue
			}
			logger.Tracef("controller added or removed, update replica now")
		case <-w.machineChanges:
			// One of the controller machines changed.
			logger.Tracef("<-w.machineChanges")
		case <-configChanges:
			// Controller config has changed.
			logger.Tracef("<-w.configChanges")

			// If a config change wakes up the loop before the topology has
			// been represented in the worker's machine trackers, ignore it;
			// errors will occur when trying to determine peer group changes.
			// Continuing is OK because subsequent invocations of the loop will
			// pick up the most recent config from state anyway.
			if len(w.machineTrackers) == 0 {
				logger.Tracef("no controller information, ignoring config change")
				continue
			}
		case requester := <-w.detailsRequests:
			// A client requested the details be resent (probably
			// because they just subscribed).
			logger.Tracef("<-w.detailsRequests (from %q)", requester)
			w.config.Hub.Publish(apiserver.DetailsTopic, w.serverDetails)
			continue
		case <-updateChan:
			// Scheduled update.
			logger.Tracef("<-updateChan")
			updateChan = nil
		}

		servers := w.apiServerHostPorts()
		apiHostPorts := make([][]network.HostPort, 0, len(servers))
		for _, serverHostPorts := range servers {
			apiHostPorts = append(apiHostPorts, serverHostPorts)
		}

		var failed bool
		if err := w.config.APIHostPortsSetter.SetAPIHostPorts(apiHostPorts); err != nil {
			logger.Errorf("cannot write API server addresses: %v", err)
			failed = true
		}

		members, err := w.updateReplicaSet()
		if err != nil {
			if _, isReplicaSetError := err.(*replicaSetError); isReplicaSetError {
				logger.Errorf("cannot set replicaset: %v", err)
			} else if _, isStepDownPrimary := err.(*stepDownPrimaryError); !isStepDownPrimary {
				return errors.Trace(err)
			}
			// both replicaset errors and stepping down the primary are both considered fast-retry 'failures'.
			// we need to re-read the state after a short timeout and re-evaluate the replicaset.
			failed = true
		}
		w.publishAPIServerDetails(servers, members)

		if failed {
			logger.Tracef("failed, waking up after: %v", retryInterval)
			updateChan = w.config.Clock.After(retryInterval)
			retryInterval = scaleRetry(retryInterval)
		} else {
			// Update the replica set members occasionally to keep them up to
			// date with the current replica-set member statuses.
			logger.Tracef("succeeded, waking up after: %v", pollInterval)
			if updateChan == nil {
				updateChan = w.config.Clock.After(pollInterval)
			}
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

// watchForControllerChanges starts two watchers for changes to controller
// info and status.
// It returns a channel which will receive events if any of the watchers fires.
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

// watchForConfigChanges starts a watcher for changes to controller config.
// It returns a channel which will receive events if the watcher fires.
// This is separate from watchForControllerChanges because of the worker loop
// logic. If controller machines have not changed, then further processing
// does not occur, whereas we want to re-publish API addresses and check
// for replica-set changes if either the management or HA space configs have
// changed.
func (w *pgWorker) watchForConfigChanges() (<-chan struct{}, error) {
	controllerConfigWatcher := w.config.State.WatchControllerConfig()
	if err := w.catacomb.Add(controllerConfigWatcher); err != nil {
		return nil, errors.Trace(err)
	}
	return controllerConfigWatcher.Changes(), nil
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
		if _, ok := w.machineTrackers[id]; ok {
			continue
		}
		logger.Debugf("found new machine %q", id)

		// Don't add the machine unless it is "Started"
		machineStatus, err := stm.Status()
		if err != nil {
			return false, errors.Annotatef(err, "cannot get status for machine %q", id)
		}
		// A machine in status Error or Stopped might still be properly running the controller. We still want to treat
		// it as an active machine, even if we're trying to tear it down.
		if machineStatus.Status != status.Pending {
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

func (w *pgWorker) apiserverDetailsRequested(topic string, request apiserver.DetailsRequest, err error) {
	if err != nil {
		// This shouldn't happen (barring programmer error ;) - treat it as fatal.
		w.catacomb.Kill(errors.Annotate(err, "apiserver details request callback failed"))
		return
	}
	select {
	case w.detailsRequests <- request.Requester:
	case <-w.catacomb.Dying():
	}
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

// publishAPIServerDetails publishes the details corresponding to the latest
// known controller/replica-set topology if it has changed from the last known
// state.
func (w *pgWorker) publishAPIServerDetails(
	servers map[string][]network.HostPort,
	members map[string]*replicaset.Member,
) {
	details := apiserver.Details{
		Servers:   make(map[string]apiserver.APIServer),
		LocalOnly: true,
	}
	internalPort := w.config.ControllerAPIPort
	if internalPort == 0 {
		internalPort = w.config.APIPort
	}
	for id, hostPorts := range servers {
		var internalAddress string
		if members[id] != nil {
			mongoAddress, _, err := net.SplitHostPort(members[id].Address)
			if err == nil {
				internalAddress = net.JoinHostPort(mongoAddress, strconv.Itoa(internalPort))
			}
		}
		server := apiserver.APIServer{
			ID:              id,
			InternalAddress: internalAddress,
		}
		for _, hp := range network.FilterUnusableHostPorts(hostPorts) {
			server.Addresses = append(server.Addresses, hp.String())
		}
		sort.Strings(server.Addresses)
		details.Servers[server.ID] = server
	}

	if !reflect.DeepEqual(w.serverDetails, details) {
		w.config.Hub.Publish(apiserver.DetailsTopic, details)
		w.serverDetails = details
	}
}

// replicaSetError holds an error returned as a result
// of calling replicaset.Set. As this is expected to fail
// in the normal course of things, it needs special treatment.
type replicaSetError struct {
	error
}

// stepDownPrimaryError means we needed to ask the primary to step down, so we should come back and re-evaluate the
// replicaset once the new primary is voted in
type stepDownPrimaryError struct {
	error
}

// updateReplicaSet sets the current replica set members, and applies the
// given voting status to machines in the state. A mapping of machine ID
// to replicaset.Member structures is returned.
func (w *pgWorker) updateReplicaSet() (map[string]*replicaset.Member, error) {
	info, err := w.peerGroupInfo()
	if err != nil {
		return nil, errors.Annotate(err, "creating peer group info")
	}
	desired, err := desiredPeerGroup(info)
	// membersChanged, members, voting, err
	if err != nil {
		return nil, errors.Annotate(err, "computing desired peer group")
	}
	if logger.IsDebugEnabled() {
		if desired.isChanged {
			logger.Debugf("desired peer group members: \n%s", prettyReplicaSetMembers(desired.members))
		} else {
			var output []string
			for id, v := range desired.machineVoting {
				output = append(output, fmt.Sprintf("  %s: %v", id, v))
			}
			logger.Debugf("no change in desired peer group, voting: \n%s", strings.Join(output, "\n"))
		}
	}

	if desired.stepDownPrimary {
		logger.Infof("mongo primary machine needs to be removed, first requesting it to step down")
		if err := w.config.MongoSession.StepDownPrimary(); err != nil {
			// StepDownPrimary should have already handled the io.EOF that mongo might give, so any error we
			// get is unknown
			return nil, errors.Annotate(err, "asking primary to step down")
		}
		// Asking the Primary to step down forces us to disconnect from Mongo, but session.Refresh() should get us
		// reconnected so we can keep operating
		w.config.MongoSession.Refresh()
		// However, we no longer know who the primary is, so we have to error out and have it reevaluated
		return nil, &stepDownPrimaryError{
			error: errors.Errorf("primary is stepping down, must reevaluate peer group"),
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
	// Iterate in obvious order so we don't get weird log messages
	votingIds := make([]string, 0, len(desired.machineVoting))
	for id := range desired.machineVoting {
		votingIds = append(votingIds, id)
	}
	sortAsInts(votingIds)
	for _, id := range votingIds {
		hasVote := desired.machineVoting[id]
		m := info.machines[id]
		switch {
		case hasVote && !m.stm.HasVote():
			added = append(added, m)
		case !hasVote && m.stm.HasVote():
			removed = append(removed, m)
		}
	}
	if err := setHasVote(added, true); err != nil {
		return nil, errors.Annotate(err, "adding new voters")
	}
	if desired.isChanged {
		ms := make([]replicaset.Member, 0, len(desired.members))
		for _, m := range desired.members {
			ms = append(ms, *m)
		}
		if err := w.config.MongoSession.Set(ms); err != nil {
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
		return nil, errors.Annotate(err, "removing non-voters")
	}

	// Reset machine status for members of the changed peer-group.
	// Any previous peer-group determination errors result in status
	// warning messages.
	for id := range desired.members {
		if err := w.machineTrackers[id].stm.SetStatus(getStatusInfo("")); err != nil {
			return nil, errors.Trace(err)
		}
	}
	for _, tracker := range info.machines {
		if tracker.stm.Life() != state.Alive && !tracker.stm.HasVote() {
			logger.Debugf("removing dying controller machine %s", tracker.Id())
			if err := w.config.State.RemoveControllerMachine(tracker.stm); err != nil {
				logger.Errorf("failed to remove dying machine as a controller after removing its vote: %v", err)
			}
		}
	}
	for _, removedTracker := range removed {
		if removedTracker.stm.Life() == state.Alive {
			logger.Debugf("vote removed from %v but machine is %s", removedTracker.Id(), state.Alive)
		}
	}
	return desired.members, nil
}

func prettyReplicaSetMembers(members map[string]*replicaset.Member) string {
	var result []string
	// Its easier to read if we sort by Id.
	keys := make([]string, 0, len(members))
	for key := range members {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		m := members[key]
		voting := "not-voting"
		if isVotingMember(m) {
			voting = "voting"
		}
		result = append(result, fmt.Sprintf("    Id: %d, Tags: %v, %s", m.Id, m.Tags, voting))
	}
	return strings.Join(result, "\n")
}

// peerGroupInfo collates current session information about the
// mongo peer group with information from state machines.
func (w *pgWorker) peerGroupInfo() (*peerGroupInfo, error) {
	sts, err := w.config.MongoSession.CurrentStatus()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get replica set status")
	}

	members, err := w.config.MongoSession.CurrentMembers()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get replica set members")
	}

	haSpace, err := w.getHASpaceFromConfig()
	if err != nil {
		return nil, err
	}

	logger.Tracef("read peer group info: %# v\n%# v", pretty.Formatter(sts), pretty.Formatter(members))
	return newPeerGroupInfo(w.machineTrackers, sts.Members, members, w.config.MongoPort, haSpace)
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
