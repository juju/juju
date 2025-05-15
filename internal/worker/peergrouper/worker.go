// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/replicaset/v3"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"github.com/kr/pretty"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/pubsub/apiserver"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.worker.peergrouper")

// ControllerConfigService is an interface for getting the controller config.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(ctx context.Context) (controller.Config, error)

	// WatchControllerConfig returns a watcher that returns keys for any changes
	// to controller config.
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

type State interface {
	RemoveControllerReference(m ControllerNode) error
	ControllerIds() ([]string, error)
	ControllerNode(id string) (ControllerNode, error)
	ControllerHost(id string) (ControllerHost, error)
	WatchControllerInfo() state.StringsWatcher
	WatchControllerStatusChanges() state.StringsWatcher
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

type MongoSession interface {
	CurrentStatus() (*replicaset.Status, error)
	CurrentMembers() ([]replicaset.Member, error)
	Set([]replicaset.Member) error
	StepDownPrimary() error
	Refresh()
}

type APIHostPortsSetter interface {
	SetAPIHostPorts(controller.Config, []network.SpaceHostPorts, []network.SpaceHostPorts) error
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
	Publish(topic string, data interface{}) (func(), error)
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
	State                   State
	ControllerConfigService ControllerConfigService
	APIHostPortsSetter      APIHostPortsSetter
	MongoSession            MongoSession
	Clock                   clock.Clock
	MongoPort               int
	APIPort                 int
	ControllerAPIPort       int

	// ControllerId is the id of the controller running this worker.
	// It is used in checking if this working is running on the
	// primary mongo node.
	ControllerId func() string

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
	if config.ControllerConfigService == nil {
		return errors.NotValidf("nil ControllerConfigService")
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
		Name: "peergrouper",
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
	if w.metrics == nil {
		return nil
	}
	return w.metrics.report()
}

func (w *pgWorker) loop() error {
	_ = w.config.PrometheusRegisterer.Register(w.metrics)
	defer w.config.PrometheusRegisterer.Unregister(w.metrics)

	controllerChanges, err := w.watchForControllerChanges()
	if err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	configChanges, err := w.watchForConfigChanges(ctx)
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

	idle := &time.Timer{}
	if w.idleFunc != nil {
		logger.Tracef(context.TODO(), "pgWorker %p set idle timeout to %s", w, IdleTime)
		idle = time.NewTimer(IdleTime)
		defer idle.Stop()
	}

	for {
		logger.Tracef(context.TODO(), "waiting...")
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-idle.C:
			logger.Tracef(context.TODO(), "pgWorker %p is idle", w)
			w.idleFunc()
			idle.Reset(IdleTime)
			continue
		case <-controllerChanges:
			// A controller was added or removed.
			logger.Tracef(context.TODO(), "<-controllerChanges")
			changed, err := w.updateControllerNodes()
			if err != nil {
				return errors.Trace(err)
			}
			if !changed {
				continue
			}
			logger.Tracef(context.TODO(), "controller added or removed, update replica now")
		case <-w.controllerChanges:
			// One of the controller nodes changed.
			logger.Tracef(context.TODO(), "<-w.controllerChanges")
		case <-configChanges:
			// Controller config has changed.
			logger.Tracef(context.TODO(), "<-w.configChanges")

			// If a config change wakes up the loop before the topology has
			// been represented in the worker's controller trackers, ignore it;
			// errors will occur when trying to determine peer group changes.
			// Continuing is OK because subsequent invocations of the loop will
			// pick up the most recent config from state anyway.
			if len(w.controllerTrackers) == 0 {
				logger.Tracef(context.TODO(), "no controller information, ignoring config change")
				continue
			}
		case requester := <-w.detailsRequests:
			// A client requested the details be resent (probably
			// because they just subscribed).
			logger.Tracef(context.TODO(), "<-w.detailsRequests (from %q)", requester)
			_, _ = w.config.Hub.Publish(apiserver.DetailsTopic, w.serverDetails)
			continue
		case <-updateChan:
			// Scheduled update.
			logger.Tracef(context.TODO(), "<-updateChan")
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
		cfg, err := w.config.ControllerConfigService.ControllerConfig(ctx)
		if err != nil {
			logger.Errorf(context.TODO(), "cannot read controller config: %v", err)
			failed = true
		}
		if err := w.config.APIHostPortsSetter.SetAPIHostPorts(cfg, apiHostPorts, apiHostPorts); err != nil {
			logger.Errorf(context.TODO(), "cannot write API server addresses: %v", err)
			failed = true
		}

		members, err := w.updateReplicaSet()
		if err != nil {
			if errors.Is(err, replicaSetError) {
				logger.Errorf(context.TODO(), "cannot set replicaset: %v", err)
			} else if !errors.Is(err, stepDownPrimaryError) {
				return errors.Trace(err)
			} else {
				logger.Tracef(context.TODO(), "isStepDownPrimary error: %v", err)
			}
			// both replicaset errors and stepping down the primary are both considered fast-retry 'failures'.
			// we need to re-read the state after a short timeout and re-evaluate the replicaset.
			failed = true
		}
		w.publishAPIServerDetails(servers, members)

		if failed {
			logger.Tracef(context.TODO(), "failed, will wake up after: %v", retryInterval)
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
				logger.Tracef(context.TODO(), "succeeded, will wake up after: %v", pollInterval)
				updateChan = w.config.Clock.After(pollInterval)
			} else {
				logger.Tracef(context.TODO(), "succeeded, wait already pending")
			}
			retryInterval = initialRetryInterval
		}
		if w.idleFunc != nil {
			idle.Reset(IdleTime)
		}
	}
}

func (w *pgWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
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
func (w *pgWorker) watchForConfigChanges(ctx context.Context) (<-chan []string, error) {
	watcher, err := w.config.ControllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return nil, errors.Trace(err)
	}

	// Consume the initial events from the watchers. The watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent[[]string](ctx, watcher); err != nil {
		return nil, errors.Trace(err)
	}

	return watcher.Changes(), nil
}

// updateControllerNodes updates the peergrouper's current list of
// controller nodes, as well as starting and stopping trackers for
// them as they are added and removed.
func (w *pgWorker) updateControllerNodes() (bool, error) {
	controllerIds, err := w.config.State.ControllerIds()
	if err != nil {
		return false, fmt.Errorf("cannot get controller ids: %v", err)
	}

	logger.Debugf(context.TODO(), "controller nodes in state: %#v", controllerIds)
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
			if errors.Is(err, errors.NotFound) {
				// If the controller isn't found, it must have been
				// removed and will soon enough be removed
				// from the controller list. This will probably
				// never happen, but we'll code defensively anyway.
				logger.Warningf(context.TODO(), "controller %q from controller list not found", id)
				continue
			}
			return false, fmt.Errorf("cannot get controller %q: %v", id, err)
		}
		controllerHost, err := w.config.State.ControllerHost(id)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				// If the controller isn't found, it must have been
				// removed and will soon enough be removed
				// from the controller list. This will probably
				// never happen, but we'll code defensively anyway.
				logger.Warningf(context.TODO(), "controller %q from controller list not found", id)
				continue
			}
			return false, fmt.Errorf("cannot get controller %q: %v", id, err)
		}
		if _, ok := w.controllerTrackers[id]; ok {
			continue
		}

		logger.Debugf(context.TODO(), "found new controller %q", id)
		tracker, err := newControllerTracker(controllerNode, controllerHost, w.controllerChanges)
		if err != nil {
			return false, errors.Trace(err)
		}
		if err := w.catacomb.Add(tracker); err != nil {
			return false, errors.Trace(err)
		}
		w.controllerTrackers[id] = tracker
		changed = true
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
			if err != nil {
				logger.Errorf(context.TODO(), "splitting host/port for address %q: %v", members[id].Address, err)
			} else {
				internalAddress = net.JoinHostPort(mongoAddress, strconv.Itoa(internalPort))
			}
		} else {
			logger.Tracef(context.TODO(), "replica-set member %q not found", id)
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

// replicaSetError means an error occurred as a result
// of calling replicaset.Set. As this is expected to fail
// in the normal course of things, it needs special treatment.
const replicaSetError = errors.ConstError("replicaset error")

// stepDownPrimaryError means we needed to ask the primary to step down, so we should come back and re-evaluate the
// replicaset once the new primary is voted in
const stepDownPrimaryError = errors.ConstError("primary is stepping down, must reevaluate peer group")

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
	if logger.IsLevelEnabled(corelogger.DEBUG) {
		if desired.isChanged {
			logger.Debugf(context.TODO(), "desired peer group members: \n%s", prettyReplicaSetMembers(desired.members))
		} else {
			var output []string
			for id, m := range desired.members {
				output = append(output, fmt.Sprintf("  %s: %v", id, isVotingMember(m)))
			}
			logger.Debugf(context.TODO(), "no change in desired peer group, voting: \n%s", strings.Join(output, "\n"))
		}
	}

	if desired.stepDownPrimary {
		logger.Infof(context.TODO(), "mongo primary controller needs to be removed, first requesting it to step down")
		if err := w.config.MongoSession.StepDownPrimary(); err != nil {
			// StepDownPrimary should have already handled the io.EOF that mongo might give, so any error we
			// get is unknown
			return nil, errors.Annotate(err, "asking primary to step down")
		}
		// Asking the Primary to step down forces us to disconnect from Mongo, but session.Refresh() should get us
		// reconnected so we can keep operating
		w.config.MongoSession.Refresh()
		// However, we no longer know who the primary is, so we have to error out and have it reevaluated
		return nil, stepDownPrimaryError
	}

	// Figure out if we are running on the mongo primary.
	controllerId := w.config.ControllerId()
	isPrimary, err := info.isPrimary(controllerId)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Annotatef(err, "determining primary status of controller %q", controllerId)
	}
	logger.Debugf(context.TODO(), "controller node %q primary: %v", controllerId, isPrimary)
	if !isPrimary {
		return desired.members, nil
	}

	// Currently k8s controllers do not support HA, so only update
	// the replicaset config if HA is enabled and there is a change.
	// Only controllers corresponding with the mongo primary should
	// update the replicaset, otherwise there will be a race since
	// a diff needs to be calculated so the changes can be applied
	// one at a time.
	if w.config.SupportsHA && desired.isChanged {
		ms := make([]replicaset.Member, 0, len(desired.members))
		ids := make([]string, 0, len(desired.members))
		for id := range desired.members {
			ids = append(ids, id)
		}
		sortAsInts(ids)
		for _, id := range ids {
			m := desired.members[id]
			ms = append(ms, *m)
		}
		// In the case of a replica set change after a primary step
		// down, the session needs to be refreshed on every other node
		// in the replica set, so that the socket addresses are updated
		// to the new primary.
		w.config.MongoSession.Refresh()
		if err := w.config.MongoSession.Set(ms); err != nil {
			return nil, errors.WithType(err, replicaSetError)
		}
		logger.Infof(context.TODO(), "successfully updated replica set")
	}

	// Reset controller status for members of the changed peer-group.
	// Any previous peer-group determination errors result in status
	// warning messages.
	for id := range desired.members {
		if err := w.controllerTrackers[id].host.SetStatus(getStatusInfo("")); err != nil {
			return nil, errors.Trace(err)
		}
	}
	if err := w.updateVoteStatus(); err != nil {
		return nil, errors.Trace(err)
	}
	for _, tracker := range w.controllerTrackers {
		if tracker.host.Life() != state.Alive && !tracker.node.HasVote() {
			logger.Debugf(context.TODO(), "removing dying controller %s references", tracker.Id())
			if err := w.config.State.RemoveControllerReference(tracker.node); err != nil {
				logger.Errorf(context.TODO(), "failed to remove dying controller as a controller after removing its vote: %v", err)
			}
		}
	}
	return desired.members, nil
}

func (w *pgWorker) updateVoteStatus() error {
	currentMembers, err := w.config.MongoSession.CurrentMembers()
	if err != nil {
		return errors.Trace(err)
	}
	orphanedNodes := set.NewStrings()
	for id := range w.controllerTrackers {
		orphanedNodes.Add(id)
	}
	var voting, nonVoting []*controllerTracker
	for _, m := range currentMembers {
		node, ok := w.controllerTrackers[m.Tags[jujuNodeKey]]
		orphanedNodes.Remove(node.Id())
		if ok {
			if !node.HasVote() && isVotingMember(&m) {
				logger.Tracef(context.TODO(), "controller %v is now voting member", node.Id())
				voting = append(voting, node)
			} else if node.HasVote() && !isVotingMember(&m) {
				logger.Tracef(context.TODO(), "controller %v is now non voting member", node.Id())
				nonVoting = append(nonVoting, node)
			}
		}
	}
	logger.Debugf(context.TODO(), "controllers that are no longer in replicaset: %v", orphanedNodes.Values())
	for _, id := range orphanedNodes.Values() {
		node := w.controllerTrackers[id]
		nonVoting = append(nonVoting, node)
	}
	if err := setHasVote(voting, true); err != nil {
		return errors.Annotatef(err, "adding voters")
	}
	if err := setHasVote(nonVoting, false); err != nil {
		return errors.Annotatef(err, "removing non-voters")
	}
	return nil
}

const (
	voting    = "voting"
	nonvoting = "non-voting"
)

func prettyReplicaSetMembers(members map[string]*replicaset.Member) string {
	var result []string
	// It's easier to read if we sort by Id.
	keys := make([]string, 0, len(members))
	for key := range members {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		m := members[key]
		voteStatus := nonvoting
		if isVotingMember(m) {
			voteStatus = voting
		}
		result = append(result, fmt.Sprintf("    Id: %d, Tags: %v, %s", m.Id, m.Tags, voteStatus))
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

	if logger.IsLevelEnabled(corelogger.TRACE) {
		logger.Tracef(context.TODO(), "read peer group info: %# v\n%# v", pretty.Formatter(sts), pretty.Formatter(members))
	}

	// If any of the trackers are for hosts still pending provisioning,
	// we disregard them. We still have trackers watching them all for changes,
	// so once they are provisioned, we will wake up and re-assess the
	// potential replica-set.
	trackers := make(map[string]*controllerTracker)
	for id, tracker := range w.controllerTrackers {
		pending, err := tracker.hostPendingProvisioning()
		if err != nil {
			return nil, errors.Trace(err)
		}

		if pending {
			logger.Infof(context.TODO(), "disregarding host pending provisioning: %q", tracker.Id())
			continue
		}

		trackers[id] = tracker
	}
	return newPeerGroupInfo(trackers, sts.Members, members, w.config.MongoPort)
}

// setHasVote sets the HasVote status of all the given nodes to hasVote.
func setHasVote(ms []*controllerTracker, hasVote bool) error {
	if len(ms) == 0 {
		return nil
	}
	logger.Infof(context.TODO(), "setting HasVote=%v on nodes %v", hasVote, ms)
	for _, m := range ms {
		if err := m.node.SetHasVote(hasVote); err != nil {
			return fmt.Errorf("cannot set voting status of %q to %v: %v", m.Id(), hasVote, err)
		}
	}
	return nil
}
