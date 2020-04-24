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

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"
	"github.com/kr/pretty"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.worker.peergrouper")

type State interface {
	RemoveControllerReference(m ControllerNode) error
	ControllerConfig() (controller.Config, error)
	ControllerIds() ([]string, error)
	ControllerNode(id string) (ControllerNode, error)
	ControllerHost(id string) (ControllerHost, error)
	WatchControllerInfo() state.StringsWatcher
	WatchControllerStatusChanges() state.StringsWatcher
	WatchControllerConfig() state.NotifyWatcher
	Space(name string) (Space, error)
}

type ControllerNode interface {
	Id() string
	Refresh() error
	Watch() state.NotifyWatcher
	WantsVote() bool
	HasVote() bool
	SetHasVote(hasVote bool) error
}

type ControllerHost interface {
	Id() string
	Life() state.Life
	Watch() state.NotifyWatcher
	Status() (status.StatusInfo, error)
	SetStatus(status.StatusInfo) error
	Refresh() error
	Addresses() network.SpaceAddresses
}

type Space interface {
	NetworkSpace() (network.SpaceInfo, error)
}

type MongoSession interface {
	CurrentStatus() (*replicaset.Status, error)
	CurrentMembers() ([]replicaset.Member, error)
	Set([]replicaset.Member) error
	StepDownPrimary() error
	Refresh()
}

type APIHostPortsSetter interface {
	SetAPIHostPorts([]network.SpaceHostPorts) error
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

	// IdleFunc allows tests to be able to get callbacks when the controller
	// hasn't been given any changes for a specified time.
	IdleFunc func()

	// IdleTime relates to how long the controller needs to wait with no changes
	// to be considered idle.
	IdleTime = 50 * time.Millisecond
)

// Hub defines the methods of the apiserver centralhub that the peer
// grouper uses.
type Hub interface {
	Subscribe(topic string, handler interface{}) (func(), error)
	Publish(topic string, data interface{}) (<-chan struct{}, error)
}

// pgWorker is a worker which watches the controller nodes in state
// as well as the MongoDB replicaset configuration, adding and
// removing controller nodes as they change or are added and
// removed.
type pgWorker struct {
	catacomb catacomb.Catacomb

	config Config

	// controllerChanges receives events from the controllerTrackers when
	// controller nodes change in ways that are relevant to the
	// peergrouper.
	controllerChanges chan struct{}

	// controllerTrackers holds the workers which track the nodes we
	// are currently watching (all the controller nodes).
	controllerTrackers map[string]*controllerTracker

	// detailsRequests is used to feed details requests from the hub into the main loop.
	detailsRequests chan string

	// serverDetails holds the last server information broadcast via pub/sub.
	// It is used to detect changes since the last publish.
	serverDetails apiserver.Details

	metrics *Collector

	idleFunc func()
}

// Config holds the configuration for a peergrouper worker.
type Config struct {
	State              State
	APIHostPortsSetter APIHostPortsSetter
	MongoSession       MongoSession
	Clock              clock.Clock
	MongoPort          int
	APIPort            int
	ControllerAPIPort  int

	// Kubernetes controllers do not support HA yet.
	SupportsHA bool

	// Hub is the central hub of the apiserver,
	// and is used to publish the details of the
	// API servers.
	Hub Hub

	PrometheusRegisterer prometheus.Registerer

	// UpdateNotify is called when the update channel is signalled.
	// Used solely for test synchronization.
	UpdateNotify func()
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
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
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
		config:             config,
		controllerChanges:  make(chan struct{}),
		controllerTrackers: make(map[string]*controllerTracker),
		detailsRequests:    make(chan string),
		idleFunc:           IdleFunc,
		metrics:            NewMetricsCollector(),
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

// Report is shown in the engine report.
func (w *pgWorker) Report() map[string]interface{} {
	return w.metrics.report()
}

func (w *pgWorker) loop() error {
	_ = w.config.PrometheusRegisterer.Register(w.metrics)
	defer w.config.PrometheusRegisterer.Unregister(w.metrics)

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

	var idle <-chan time.Time
	if w.idleFunc != nil {
		logger.Tracef("pgWorker %p set idle timeout to %s", w, IdleTime)
		idle = time.After(IdleTime)
	}

	for {
		logger.Tracef("waiting...")
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-idle:
			logger.Tracef("pgWorker %p is idle", w)
			w.idleFunc()
			idle = time.After(IdleTime)
			continue
		case <-controllerChanges:
			// A controller controller was added or removed.
			logger.Tracef("<-controllerChanges")
			changed, err := w.updateControllerNodes()
			if err != nil {
				return errors.Trace(err)
			}
			if !changed {
				continue
			}
			logger.Tracef("controller added or removed, update replica now")
		case <-w.controllerChanges:
			// One of the controller nodes changed.
			logger.Tracef("<-w.controllerChanges")
		case <-configChanges:
			// Controller config has changed.
			logger.Tracef("<-w.configChanges")

			// If a config change wakes up the loop before the topology has
			// been represented in the worker's controller trackers, ignore it;
			// errors will occur when trying to determine peer group changes.
			// Continuing is OK because subsequent invocations of the loop will
			// pick up the most recent config from state anyway.
			if len(w.controllerTrackers) == 0 {
				logger.Tracef("no controller information, ignoring config change")
				continue
			}
		case requester := <-w.detailsRequests:
			// A client requested the details be resent (probably
			// because they just subscribed).
			logger.Tracef("<-w.detailsRequests (from %q)", requester)
			_, _ = w.config.Hub.Publish(apiserver.DetailsTopic, w.serverDetails)
			continue
		case <-updateChan:
			// Scheduled update.
			logger.Tracef("<-updateChan")
			updateChan = nil
			if w.config.UpdateNotify != nil {
				w.config.UpdateNotify()
			}
		}

		servers := w.apiServerHostPorts()
		apiHostPorts := make([]network.SpaceHostPorts, 0, len(servers))
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
			} else {
				logger.Tracef("isStepDownPrimary error: %v", err)
			}
			// both replicaset errors and stepping down the primary are both considered fast-retry 'failures'.
			// we need to re-read the state after a short timeout and re-evaluate the replicaset.
			failed = true
		}
		w.publishAPIServerDetails(servers, members)

		if failed {
			logger.Tracef("failed, will wake up after: %v", retryInterval)
			updateChan = w.config.Clock.After(retryInterval)
			retryInterval = scaleRetry(retryInterval)
		} else {
			// Update the replica set members occasionally to keep them up to
			// date with the current replica-set member statuses.
			// If we had previously failed to update the replicaset,
			// the updateChan isn't set to the pollInterval. So if we had just
			// processed an update, or have just succeeded after a failure reset
			// the updateChan to the pollInterval.
			if updateChan == nil || retryInterval != initialRetryInterval {
				logger.Tracef("succeeded, will wake up after: %v", pollInterval)
				updateChan = w.config.Clock.After(pollInterval)
			} else {
				logger.Tracef("succeeded, wait already pending")
			}
			retryInterval = initialRetryInterval
		}
		if w.idleFunc != nil {
			idle = time.After(IdleTime)
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
	var notifyCh chan struct{}
	go func() {
		for {
			select {
			case <-w.catacomb.Dying():
				return
			case <-controllerInfoWatcher.Changes():
				notifyCh = out
			case <-controllerStatusWatcher.Changes():
				notifyCh = out
			case notifyCh <- struct{}{}:
				notifyCh = nil
			}
		}
	}()
	return out, nil
}

// watchForConfigChanges starts a watcher for changes to controller config.
// It returns a channel which will receive events if the watcher fires.
// This is separate from watchForControllerChanges because of the worker loop
// logic. If controller nodes have not changed, then further processing
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

// updateControllerNodes updates the peergrouper's current list of
// controller nodes, as well as starting and stopping trackers for
// them as they are added and removed.
func (w *pgWorker) updateControllerNodes() (bool, error) {
	controllerIds, err := w.config.State.ControllerIds()
	if err != nil {
		return false, fmt.Errorf("cannot get controller ids: %v", err)
	}

	logger.Debugf("controller nodes in state: %#v", controllerIds)
	changed := false

	// Stop controller goroutines that no longer correspond to controller nodes.
	for _, m := range w.controllerTrackers {
		if !inStrings(m.Id(), controllerIds) {
			_ = worker.Stop(m)
			delete(w.controllerTrackers, m.Id())
			changed = true
		}
	}

	// Start nodes with no watcher
	for _, id := range controllerIds {
		controllerNode, err := w.config.State.ControllerNode(id)
		if err != nil {
			if errors.IsNotFound(err) {
				// If the controller isn't found, it must have been
				// removed and will soon enough be removed
				// from the controller list. This will probably
				// never happen, but we'll code defensively anyway.
				logger.Warningf("controller %q from controller list not found", id)
				continue
			}
			return false, fmt.Errorf("cannot get controller %q: %v", id, err)
		}
		controllerHost, err := w.config.State.ControllerHost(id)
		if err != nil {
			if errors.IsNotFound(err) {
				// If the controller isn't found, it must have been
				// removed and will soon enough be removed
				// from the controller list. This will probably
				// never happen, but we'll code defensively anyway.
				logger.Warningf("controller %q from controller list not found", id)
				continue
			}
			return false, fmt.Errorf("cannot get controller %q: %v", id, err)
		}
		if _, ok := w.controllerTrackers[id]; ok {
			continue
		}
		logger.Debugf("found new controller %q", id)

		// Don't add the controller unless it is "Started"
		nodeStatus, err := controllerHost.Status()
		if err != nil {
			return false, errors.Annotatef(err, "cannot get status for controller %q", id)
		}
		// A controller in status Error or Stopped might still be properly running the controller. We still want to treat
		// it as an active controller, even if we're trying to tear it down.
		if nodeStatus.Status != status.Pending {
			logger.Debugf("controller %q has started, adding it to peergrouper list", id)
			tracker, err := newControllerTracker(controllerNode, controllerHost, w.controllerChanges)
			if err != nil {
				return false, errors.Trace(err)
			}
			if err := w.catacomb.Add(tracker); err != nil {
				return false, errors.Trace(err)
			}
			w.controllerTrackers[id] = tracker
			changed = true
		} else {
			logger.Debugf("controller %q not ready: %v", id, nodeStatus.Status)
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

// apiServerHostPorts returns the host-ports for each apiserver controller.
func (w *pgWorker) apiServerHostPorts() map[string]network.SpaceHostPorts {
	servers := make(map[string]network.SpaceHostPorts)
	for _, m := range w.controllerTrackers {
		hostPorts := network.SpaceAddressesWithPort(m.Addresses(), w.config.APIPort)
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
	servers map[string]network.SpaceHostPorts,
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
		for _, hp := range hostPorts.HostPorts().FilterUnusable() {
			server.Addresses = append(server.Addresses, network.DialAddress(hp))
		}
		sort.Strings(server.Addresses)
		details.Servers[server.ID] = server
	}

	if !reflect.DeepEqual(w.serverDetails, details) {
		_, _ = w.config.Hub.Publish(apiserver.DetailsTopic, details)
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
// given voting status to nodes in the state. A mapping of controller ID
// to replicaset.Member structures is returned.
func (w *pgWorker) updateReplicaSet() (map[string]*replicaset.Member, error) {
	info, err := w.peerGroupInfo()
	if err != nil {
		return nil, errors.Annotate(err, "creating peer group info")
	}
	// Update the metrics collector with the replicaset statuses.
	w.metrics.update(info.statuses)
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
			for id, v := range desired.nodeVoting {
				output = append(output, fmt.Sprintf("  %s: %v", id, v))
			}
			logger.Debugf("no change in desired peer group, voting: \n%s", strings.Join(output, "\n"))
		}
	}

	if desired.stepDownPrimary {
		logger.Infof("mongo primary controller needs to be removed, first requesting it to step down")
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

	// We cannot change the HasVote flag of a controller in state at exactly
	// the same moment as changing its voting status in the replica set.
	//
	// Thus we need to be careful that a controller which is actually a voting
	// member is not seen to not have a vote, because otherwise
	// there is nothing to prevent the controller being removed.
	//
	// To avoid this happening, we make sure when we call SetReplicaSet,
	// that the voting status of nodes is the union of both old
	// and new voting nodes - that is the set of HasVote nodes
	// is a superset of all the actual voting nodes.
	//
	// Only after the call has taken place do we reset the voting status
	// of the nodes that have lost their vote.
	//
	// If there's a crash, the voting status may not reflect the
	// actual voting status for a while, but when things come
	// back on line, it will be sorted out, as desiredReplicaSet
	// will return the actual voting status.
	//
	// Note that we potentially update the HasVote status of the nodes even
	// if the members have not changed.
	var added, removed []*controllerTracker
	// Iterate in obvious order so we don't get weird log messages
	votingIds := make([]string, 0, len(desired.nodeVoting))
	for id := range desired.nodeVoting {
		votingIds = append(votingIds, id)
	}
	sortAsInts(votingIds)
	for _, id := range votingIds {
		hasVote := desired.nodeVoting[id]
		m := info.controllers[id]
		switch {
		case hasVote && !m.node.HasVote():
			added = append(added, m)
		case !hasVote && m.node.HasVote():
			removed = append(removed, m)
		}
	}
	if err := setHasVote(added, true); err != nil {
		return nil, errors.Annotate(err, "adding new voters")
	}
	// Currently k8s controllers do not support HA, so only update
	// the replicaset config if HA is enabled and there is a change.
	if w.config.SupportsHA && desired.isChanged {
		ms := make([]replicaset.Member, 0, len(desired.members))
		for _, m := range desired.members {
			ms = append(ms, *m)
		}
		if err := w.config.MongoSession.Set(ms); err != nil {
			// We've failed to set the replica set, so revert back
			// to the previous settings.
			if err1 := setHasVote(added, false); err1 != nil {
				logger.Errorf("cannot revert controller voting after failure to change replica set: %v", err1)
			}
			return nil, &replicaSetError{err}
		}
		logger.Infof("successfully updated replica set")
	}
	if err := setHasVote(removed, false); err != nil {
		return nil, errors.Annotate(err, "removing non-voters")
	}

	// Reset controller status for members of the changed peer-group.
	// Any previous peer-group determination errors result in status
	// warning messages.
	for id := range desired.members {
		if err := w.controllerTrackers[id].host.SetStatus(getStatusInfo("")); err != nil {
			return nil, errors.Trace(err)
		}
	}
	for _, tracker := range info.controllers {
		if tracker.host.Life() != state.Alive && !tracker.node.HasVote() {
			logger.Debugf("removing dying controller %s", tracker.Id())
			if err := w.config.State.RemoveControllerReference(tracker.node); err != nil {
				logger.Errorf("failed to remove dying controller as a controller after removing its vote: %v", err)
			}
		}
	}
	for _, removedTracker := range removed {
		if removedTracker.host.Life() == state.Alive {
			logger.Debugf("vote removed from %v but controller is %s", removedTracker.Id(), state.Alive)
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
// mongo peer group with information from state node instances.
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
	return newPeerGroupInfo(w.controllerTrackers, sts.Members, members, w.config.MongoPort, haSpace)
}

// getHASpaceFromConfig returns a space based on the controller's
// configuration for the HA space.
func (w *pgWorker) getHASpaceFromConfig() (network.SpaceInfo, error) {
	config, err := w.config.State.ControllerConfig()
	if err != nil {
		return network.SpaceInfo{}, errors.Trace(err)
	}

	jujuHASpace := config.JujuHASpace()
	if jujuHASpace == "" {
		return network.SpaceInfo{}, nil
	}
	space, err := w.config.State.Space(jujuHASpace)
	if err != nil {
		return network.SpaceInfo{}, errors.Trace(err)
	}
	return space.NetworkSpace()
}

// setHasVote sets the HasVote status of all the given nodes to hasVote.
func setHasVote(ms []*controllerTracker, hasVote bool) error {
	if len(ms) == 0 {
		return nil
	}
	logger.Infof("setting HasVote=%v on nodes %v", hasVote, ms)
	for _, m := range ms {
		if err := m.node.SetHasVote(hasVote); err != nil {
			return fmt.Errorf("cannot set voting status of %q to %v: %v", m.Id(), hasVote, err)
		}
	}
	return nil
}
