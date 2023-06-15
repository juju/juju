// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	jworker "github.com/juju/juju/worker"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one in the RemoteStateWatcher.
type logger interface{}

var _ logger = struct{}{}

// Logger represents the logging methods used in this package.
type Logger interface {
	Warningf(string, ...interface{})
	Debugf(string, ...interface{})
	Criticalf(string, ...interface{})
}

// SecretTriggerWatcherFunc is a function returning a secrets trigger watcher.
type SecretTriggerWatcherFunc func(names.UnitTag, bool, chan []string) (worker.Worker, error)

// SecretsClient provides access to the secrets manager facade.
type SecretsClient interface {
	WatchConsumedSecretsChanges(unitName string) (watcher.StringsWatcher, error)
	GetConsumerSecretsRevisionInfo(string, []string) (map[string]secrets.SecretRevisionInfo, error)
	WatchObsolete(ownerTags ...names.Tag) (watcher.StringsWatcher, error)
}

// RemoteStateWatcher collects unit, application, and application config information
// from separate state watchers, and updates a Snapshot which is sent on a
// channel upon change.
type RemoteStateWatcher struct {
	st                           State
	unit                         Unit
	application                  Application
	modelType                    model.ModelType
	sidecar                      bool
	enforcedCharmModifiedVersion int
	logger                       Logger

	relations                     map[names.RelationTag]*wrappedRelationUnitsWatcher
	relationUnitsChanges          chan relationUnitsChange
	storageAttachmentWatchers     map[names.StorageTag]*storageAttachmentWatcher
	storageAttachmentChanges      chan storageAttachmentChange
	leadershipTracker             leadership.Tracker
	updateStatusChannel           UpdateStatusTimerFunc
	commandChannel                <-chan string
	retryHookChannel              watcher.NotifyChannel
	containerRunningStatusChannel watcher.NotifyChannel
	containerRunningStatusFunc    ContainerRunningStatusFunc
	canApplyCharmProfile          bool
	workloadEventChannel          <-chan string
	shutdownChannel               <-chan bool

	secretsClient SecretsClient

	secretRotateWatcherFunc SecretTriggerWatcherFunc
	secretRotateWatcher     worker.Worker
	rotateSecretsChanges    chan []string

	secretExpiryWatcherFunc SecretTriggerWatcherFunc
	secretExpiryWatcher     worker.Worker
	expireSecretsChanges    chan []string

	obsoleteRevisionWatcher worker.Worker
	obsoleteRevisionChanges watcher.StringsChannel

	catacomb catacomb.Catacomb

	out     chan struct{}
	mu      sync.Mutex
	current Snapshot
}

// ContainerRunningStatus is used on CAAS models to upgrade charms/block actions.
type ContainerRunningStatus struct {
	PodName          string
	Initialising     bool
	InitialisingTime time.Time
	Running          bool
}

// ContainerRunningStatusFunc is used by the RemoteStateWatcher in a CAAS
// model to determine if the unit is running and ready to execute actions.
type ContainerRunningStatusFunc func(providerID string) (*ContainerRunningStatus, error)

// WatcherConfig holds configuration parameters for the
// remote state watcher.
type WatcherConfig struct {
	State                         State
	LeadershipTracker             leadership.Tracker
	SecretRotateWatcherFunc       SecretTriggerWatcherFunc
	SecretExpiryWatcherFunc       SecretTriggerWatcherFunc
	SecretsClient                 SecretsClient
	UpdateStatusChannel           UpdateStatusTimerFunc
	CommandChannel                <-chan string
	RetryHookChannel              watcher.NotifyChannel
	ContainerRunningStatusChannel watcher.NotifyChannel
	ContainerRunningStatusFunc    ContainerRunningStatusFunc
	UnitTag                       names.UnitTag
	ModelType                     model.ModelType
	Sidecar                       bool
	EnforcedCharmModifiedVersion  int
	Logger                        Logger
	CanApplyCharmProfile          bool
	WorkloadEventChannel          <-chan string
	InitialWorkloadEventIDs       []string
	ShutdownChannel               <-chan bool
}

func (w WatcherConfig) validate() error {
	if w.ModelType == model.IAAS && w.Sidecar {
		return errors.NewNotValid(nil, fmt.Sprintf("sidecar mode is only for %q model", model.CAAS))
	}

	if w.ModelType == model.CAAS && !w.Sidecar {
		if w.ContainerRunningStatusChannel != nil &&
			w.ContainerRunningStatusFunc == nil {
			return errors.NotValidf("watcher config for CAAS model with nil container running status func")
		}
	}
	if w.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewWatcher returns a RemoteStateWatcher that handles state changes pertaining to the
// supplied unit.
func NewWatcher(config WatcherConfig) (*RemoteStateWatcher, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &RemoteStateWatcher{
		st:                            config.State,
		relations:                     make(map[names.RelationTag]*wrappedRelationUnitsWatcher),
		relationUnitsChanges:          make(chan relationUnitsChange),
		storageAttachmentWatchers:     make(map[names.StorageTag]*storageAttachmentWatcher),
		storageAttachmentChanges:      make(chan storageAttachmentChange),
		leadershipTracker:             config.LeadershipTracker,
		secretRotateWatcherFunc:       config.SecretRotateWatcherFunc,
		secretExpiryWatcherFunc:       config.SecretExpiryWatcherFunc,
		secretsClient:                 config.SecretsClient,
		updateStatusChannel:           config.UpdateStatusChannel,
		commandChannel:                config.CommandChannel,
		retryHookChannel:              config.RetryHookChannel,
		containerRunningStatusChannel: config.ContainerRunningStatusChannel,
		containerRunningStatusFunc:    config.ContainerRunningStatusFunc,
		modelType:                     config.ModelType,
		logger:                        config.Logger,
		canApplyCharmProfile:          config.CanApplyCharmProfile,
		// Note: it is important that the out channel be buffered!
		// The remote state watcher will perform a non-blocking send
		// on the channel to wake up the observer. It is non-blocking
		// so that we coalesce events while the observer is busy.
		out: make(chan struct{}, 1),
		current: Snapshot{
			Relations:               make(map[int]RelationSnapshot),
			Storage:                 make(map[names.StorageTag]StorageSnapshot),
			ActionsBlocked:          config.ContainerRunningStatusChannel != nil,
			ActionChanged:           make(map[string]int),
			UpgradeMachineStatus:    model.UpgradeSeriesNotStarted,
			WorkloadEvents:          config.InitialWorkloadEventIDs,
			ConsumedSecretInfo:      make(map[string]secrets.SecretRevisionInfo),
			ObsoleteSecretRevisions: make(map[string][]int),
		},
		sidecar:                      config.Sidecar,
		enforcedCharmModifiedVersion: config.EnforcedCharmModifiedVersion,
		workloadEventChannel:         config.WorkloadEventChannel,
		shutdownChannel:              config.ShutdownChannel,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			return w.loop(config.UnitTag)
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *RemoteStateWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *RemoteStateWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *RemoteStateWatcher) RemoteStateChanged() <-chan struct{} {
	return w.out
}

func (w *RemoteStateWatcher) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	snapshot := w.current
	snapshot.Relations = make(map[int]RelationSnapshot)
	for id, relationSnapshot := range w.current.Relations {
		relationSnapshotCopy := RelationSnapshot{
			Life:               relationSnapshot.Life,
			Suspended:          relationSnapshot.Suspended,
			Members:            make(map[string]int64),
			ApplicationMembers: make(map[string]int64),
		}
		for name, version := range relationSnapshot.Members {
			relationSnapshotCopy.Members[name] = version
		}
		for name, version := range relationSnapshot.ApplicationMembers {
			relationSnapshotCopy.ApplicationMembers[name] = version
		}
		snapshot.Relations[id] = relationSnapshotCopy
	}
	snapshot.Storage = make(map[names.StorageTag]StorageSnapshot)
	for tag, storageSnapshot := range w.current.Storage {
		snapshot.Storage[tag] = storageSnapshot
	}
	snapshot.ActionsPending = make([]string, len(w.current.ActionsPending))
	copy(snapshot.ActionsPending, w.current.ActionsPending)
	snapshot.Commands = make([]string, len(w.current.Commands))
	copy(snapshot.Commands, w.current.Commands)
	snapshot.WorkloadEvents = make([]string, len(w.current.WorkloadEvents))
	copy(snapshot.WorkloadEvents, w.current.WorkloadEvents)
	snapshot.ActionChanged = make(map[string]int)
	for k, v := range w.current.ActionChanged {
		snapshot.ActionChanged[k] = v
	}
	snapshot.SecretRotations = make([]string, len(w.current.SecretRotations))
	copy(snapshot.SecretRotations, w.current.SecretRotations)
	snapshot.ConsumedSecretInfo = make(map[string]secrets.SecretRevisionInfo)
	for u, r := range w.current.ConsumedSecretInfo {
		snapshot.ConsumedSecretInfo[u] = r
	}
	snapshot.ObsoleteSecretRevisions = make(map[string][]int)
	for u, r := range w.current.ObsoleteSecretRevisions {
		rCopy := make([]int, len(r))
		copy(rCopy, r)
		snapshot.ObsoleteSecretRevisions[u] = rCopy
	}
	snapshot.DeletedSecrets = make([]string, len(w.current.DeletedSecrets))
	copy(snapshot.DeletedSecrets, w.current.DeletedSecrets)
	return snapshot
}

func (w *RemoteStateWatcher) ClearResolvedMode() {
	w.mu.Lock()
	w.current.ResolvedMode = params.ResolvedNone
	w.mu.Unlock()
}

func (w *RemoteStateWatcher) CommandCompleted(completed string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, id := range w.current.Commands {
		if id != completed {
			continue
		}
		w.current.Commands = append(
			w.current.Commands[:i],
			w.current.Commands[i+1:]...,
		)
		break
	}
}

func (w *RemoteStateWatcher) WorkloadEventCompleted(workloadEventID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, id := range w.current.WorkloadEvents {
		if id != workloadEventID {
			continue
		}
		w.current.WorkloadEvents = append(
			w.current.WorkloadEvents[:i],
			w.current.WorkloadEvents[i+1:]...,
		)
		break
	}
}

// RotateSecretCompleted is called when a secret identified by the URL
// has been rotated.
func (w *RemoteStateWatcher) RotateSecretCompleted(rotatedURL string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, url := range w.current.SecretRotations {
		if url != rotatedURL {
			continue
		}
		w.current.SecretRotations = append(
			w.current.SecretRotations[:i],
			w.current.SecretRotations[i+1:]...,
		)
		break
	}
}

// ExpireRevisionCompleted is called when a secret revision
// has been expired.
func (w *RemoteStateWatcher) ExpireRevisionCompleted(expiredRevision string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, rev := range w.current.ExpiredSecretRevisions {
		if rev != expiredRevision {
			continue
		}
		w.current.ExpiredSecretRevisions = append(
			w.current.ExpiredSecretRevisions[:i],
			w.current.ExpiredSecretRevisions[i+1:]...,
		)
		break
	}
}

// RemoveSecretsCompleted is called when secrets have been deleted.
func (w *RemoteStateWatcher) RemoveSecretsCompleted(uris []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	deleted := set.NewStrings(uris...)
	currentDeleted := set.NewStrings(w.current.DeletedSecrets...)
	w.current.DeletedSecrets = currentDeleted.Difference(deleted).Values()
}

func (w *RemoteStateWatcher) setUp(unitTag names.UnitTag) (err error) {
	// TODO(axw) move this logic
	defer func() {
		cause := errors.Cause(err)
		if params.IsCodeNotFoundOrCodeUnauthorized(cause) {
			// We only want to terminate the agent for IAAS models.
			if w.modelType == model.IAAS {
				err = jworker.ErrTerminateAgent
			}
		}
	}()
	if w.unit, err = w.st.Unit(unitTag); err != nil {
		return errors.Trace(err)
	}
	w.application, err = w.unit.Application()
	if err != nil {
		return errors.Trace(err)
	}
	if w.containerRunningStatusFunc != nil {
		providerID := w.unit.ProviderID()
		if providerID != "" {
			running, err := w.containerRunningStatusFunc(providerID)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			if running != nil {
				w.containerRunningStatus(*running)
			}
		}
	}
	return nil
}

func (w *RemoteStateWatcher) loop(unitTag names.UnitTag) (err error) {
	if err := w.setUp(unitTag); err != nil {
		return errors.Trace(err)
	}

	var requiredEvents int

	var seenUnitChange bool
	unitw, err := w.unit.Watch()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(unitw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenConfigChange bool
	charmConfigw, err := w.unit.WatchConfigSettingsHash()
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.catacomb.Add(charmConfigw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenTrustConfigChange bool
	trustConfigw, err := w.unit.WatchTrustConfigSettingsHash()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(trustConfigw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenRelationsChange bool
	relationsw, err := w.unit.WatchRelations()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(relationsw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenAddressesChange bool
	addressesw, err := w.unit.WatchAddressesHash()
	if err != nil {
		return errors.Trace(err)
	}
	addressesChanges := addressesw.Changes()
	if err := w.catacomb.Add(addressesw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenSecretsChange bool
	secretsw, err := w.secretsClient.WatchConsumedSecretsChanges(w.unit.Tag().Id())
	if err != nil {
		return errors.Trace(err)
	}
	secretsChanges := secretsw.Changes()
	if err := w.catacomb.Add(secretsw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var (
		seenApplicationChange   bool
		seenInstanceDataChange  bool
		seenUpgradeSeriesChange bool
		upgradeSeriesChanges    watcher.NotifyChannel
		instanceDataChannel     watcher.NotifyChannel
	)

	applicationw, err := w.application.Watch()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(applicationw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	if w.modelType == model.IAAS {
		// Only IAAS models support upgrading the machine series.
		// TODO(externalreality) This pattern should probably be extracted
		upgradeSeriesw, err := w.unit.WatchUpgradeSeriesNotifications()
		if err != nil {
			return errors.Trace(err)
		}
		if err := w.catacomb.Add(upgradeSeriesw); err != nil {
			return errors.Trace(err)
		}
		upgradeSeriesChanges = upgradeSeriesw.Changes()
		requiredEvents++
	}

	if w.canApplyCharmProfile {
		// Note: canApplyCharmProfile will be false for a CAAS model.
		instanceDataW, err := w.unit.WatchInstanceData()
		if err != nil {
			return errors.Trace(err)
		}
		if err := w.catacomb.Add(instanceDataW); err != nil {
			return errors.Trace(err)
		}
		instanceDataChannel = instanceDataW.Changes()
		requiredEvents++
	}

	var seenStorageChange bool
	storagew, err := w.unit.WatchStorage()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(storagew); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenLeaderSettingsChange bool
	leaderSettingsw, err := w.application.WatchLeadershipSettings()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(leaderSettingsw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenActionsChange bool
	actionsw, err := w.unit.WatchActionNotifications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(actionsw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenUpdateStatusIntervalChange bool
	updateStatusIntervalw, err := w.st.WatchUpdateStatusHookInterval()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(updateStatusIntervalw); err != nil {
		return errors.Trace(err)
	}
	requiredEvents++

	var seenLeadershipChange bool
	// There's no watcher for this per se; we wait on a channel
	// returned by the leadership tracker.
	requiredEvents++

	var eventsObserved int
	observedEvent := func(flag *bool) {
		if flag == nil || !*flag {
			*flag = true
			eventsObserved++
		}
	}

	// fire will, once the first event for each watcher has
	// been observed, send a signal on the out channel.
	fire := func() {
		if eventsObserved != requiredEvents {
			return
		}
		select {
		case w.out <- struct{}{}:
		default:
		}
	}

	// Check the initial leadership status, and then we can flip-flop
	// waiting on leader or minion to trigger the changed event.
	var waitLeader, waitMinion <-chan struct{}
	claimLeader := w.leadershipTracker.ClaimLeader()
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case <-claimLeader.Ready():
		isLeader := claimLeader.Wait()
		if err := w.leadershipChanged(isLeader); err != nil {
			return errors.Trace(err)
		}
		if isLeader {
			waitMinion = w.leadershipTracker.WaitMinion().Ready()
		} else {
			waitLeader = w.leadershipTracker.WaitLeader().Ready()
		}
		observedEvent(&seenLeadershipChange)
	}

	var updateStatusInterval time.Duration
	var updateStatusTimer <-chan time.Time
	resetUpdateStatusTimer := func() {
		updateStatusTimer = w.updateStatusChannel(updateStatusInterval).After()
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-unitw.Changes():
			w.logger.Debugf("got unit change for %s", w.unit.Tag().Id())
			if !ok {
				return errors.New("unit watcher closed")
			}
			if err := w.unitChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenUnitChange)

		case _, ok := <-applicationw.Changes():
			w.logger.Debugf("got application change for %s", w.unit.Tag().Id())
			if !ok {
				return errors.New("application watcher closed")
			}
			if err := w.applicationChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenApplicationChange)

		case secrets, ok := <-secretsChanges:
			w.logger.Debugf("got secrets change for %s: %s", w.unit.Tag().Id(), secrets)
			if !ok {
				return errors.New("secrets watcher closed")
			}
			if err := w.secretsChanged(secrets); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenSecretsChange)

		case _, ok := <-instanceDataChannel:
			w.logger.Debugf("got instance data change for %s", w.unit.Tag().Id())
			if !ok {
				return errors.New("instance data watcher closed")
			}
			if err := w.instanceDataChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenInstanceDataChange)

		case _, ok := <-w.containerRunningStatusChannel:
			w.logger.Debugf("got running status change for %s", w.unit.Tag().Id())
			if !ok {
				return errors.New("running status watcher closed")
			}
			if w.current.ProviderID == "" {
				if err := w.unitChanged(); err != nil {
					return errors.Trace(err)
				}
				if w.current.ProviderID == "" {
					// This shouldn't happen.
					w.logger.Warningf("we should already be assigned a provider id for %s but got an empty id", w.unit.Tag().Id())
					return nil
				}
			}
			runningStatus, err := w.containerRunningStatusFunc(w.current.ProviderID)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Annotatef(err, "getting container running status for %q", unitTag.String())
			}
			if runningStatus != nil {
				w.containerRunningStatus(*runningStatus)
			}

		case hashes, ok := <-charmConfigw.Changes():
			w.logger.Debugf("got config change for %s: ok=%t, hashes=%v", w.unit.Tag().Id(), ok, hashes)
			if !ok {
				return errors.New("config watcher closed")
			}
			if len(hashes) != 1 {
				return errors.New("expected one hash in config change")
			}
			w.configHashChanged(hashes[0])
			observedEvent(&seenConfigChange)

		case hashes, ok := <-trustConfigw.Changes():
			w.logger.Debugf("got trust config change for %s: ok=%t, hashes=%v", w.unit.Tag().Id(), ok, hashes)
			if !ok {
				return errors.New("trust config watcher closed")
			}
			if len(hashes) != 1 {
				return errors.New("expected one hash in trust config change")
			}
			w.trustHashChanged(hashes[0])
			observedEvent(&seenTrustConfigChange)

		case _, ok := <-upgradeSeriesChanges:
			w.logger.Debugf("got upgrade series change")
			if !ok {
				return errors.New("upgrades series watcher closed")
			}
			if err := w.upgradeSeriesStatusChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenUpgradeSeriesChange)

		case hashes, ok := <-addressesChanges:
			w.logger.Debugf("got address change for %s: ok=%t, hashes=%v", w.unit.Tag().Id(), ok, hashes)
			if !ok {
				return errors.New("addresses watcher closed")
			}
			if len(hashes) != 1 {
				return errors.New("expected one hash in addresses change")
			}
			w.addressesHashChanged(hashes[0])
			observedEvent(&seenAddressesChange)

		case _, ok := <-leaderSettingsw.Changes():
			w.logger.Debugf("got leader settings change for %s: ok=%t", w.unit.Tag().Id(), ok)
			if !ok {
				return errors.New("leader settings watcher closed")
			}
			if err := w.leaderSettingsChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenLeaderSettingsChange)

		case actions, ok := <-actionsw.Changes():
			w.logger.Debugf("got action change for %s: %v ok=%t", w.unit.Tag().Id(), actions, ok)
			if !ok {
				return errors.New("actions watcher closed")
			}
			w.actionsChanged(actions)
			observedEvent(&seenActionsChange)

		case keys, ok := <-relationsw.Changes():
			w.logger.Debugf("got relations change for %s: ok=%t", w.unit.Tag().Id(), ok)
			if !ok {
				return errors.New("relations watcher closed")
			}
			if err := w.relationsChanged(keys); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenRelationsChange)

		case keys, ok := <-storagew.Changes():
			w.logger.Debugf("got storage change for %s: %v ok=%t", w.unit.Tag().Id(), keys, ok)
			if !ok {
				return errors.New("storage watcher closed")
			}
			if err := w.storageChanged(keys); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenStorageChange)

		case _, ok := <-updateStatusIntervalw.Changes():
			w.logger.Debugf("got update status interval change for %s: ok=%t", w.unit.Tag().Id(), ok)
			if !ok {
				return errors.New("update status interval watcher closed")
			}
			observedEvent(&seenUpdateStatusIntervalChange)

			var err error
			updateStatusInterval, err = w.st.UpdateStatusHookInterval()
			if err != nil {
				return errors.Trace(err)
			}
			wasActive := updateStatusTimer != nil
			resetUpdateStatusTimer()
			if wasActive {
				// This is not the first time we've seen an update
				// status interval change, so there's no need to
				// fall out and fire an initial change event.
				continue
			}

		case <-waitMinion:
			w.logger.Debugf("got leadership change for %v: minion", unitTag.Id())
			if err := w.leadershipChanged(false); err != nil {
				return errors.Trace(err)
			}
			waitMinion = nil
			waitLeader = w.leadershipTracker.WaitLeader().Ready()

		case <-waitLeader:
			w.logger.Debugf("got leadership change for %v: leader", unitTag.Id())
			if err := w.leadershipChanged(true); err != nil {
				return errors.Trace(err)
			}
			waitLeader = nil
			waitMinion = w.leadershipTracker.WaitMinion().Ready()

		case uris, ok := <-w.rotateSecretsChanges:
			if !ok || len(uris) == 0 {
				continue
			}
			w.logger.Debugf("got rotate secret URIs: %q", uris)
			w.rotateSecretURIs(uris)

		case revisions, ok := <-w.expireSecretsChanges:
			if !ok || len(revisions) == 0 {
				continue
			}
			w.logger.Debugf("got expired secret revisions: %q", revisions)
			w.expireSecretRevisions(revisions)

		case secretRevisions, ok := <-w.obsoleteRevisionChanges:
			w.logger.Debugf("got obsolete secret revisions change for %s: %s", w.application.Tag().Id(), secretRevisions)
			if !ok {
				return errors.New("secret revisions watcher closed")
			}
			if err := w.secretObsoleteRevisionsChanged(secretRevisions); err != nil {
				return errors.Trace(err)
			}

		case change := <-w.storageAttachmentChanges:
			w.logger.Debugf("storage attachment change for %s: %v", w.unit.Tag().Id(), change)
			w.storageAttachmentChanged(change)

		case change := <-w.relationUnitsChanges:
			w.logger.Debugf("got a relation units change for %s : %v", w.unit.Tag().Id(), change)
			if err := w.relationUnitsChanged(change); err != nil {
				return errors.Trace(err)
			}

		case <-updateStatusTimer:
			w.logger.Debugf("update status timer triggered for %s", w.unit.Tag().Id())
			w.updateStatusChanged()
			resetUpdateStatusTimer()

		case id, ok := <-w.commandChannel:
			if !ok {
				return errors.New("commandChannel closed")
			}
			w.logger.Debugf("command enqueued for %s: %v", w.unit.Tag().Id(), id)
			w.commandsChanged(id)

		case id, ok := <-w.workloadEventChannel:
			if !ok {
				return errors.New("workloadEventChannel closed")
			}
			w.logger.Debugf("workloadEvent enqueued for %s: %v", w.unit.Tag().Id(), id)
			w.workloadEventsChanged(id)

		case _, ok := <-w.retryHookChannel:
			if !ok {
				return errors.New("retryHookChannel closed")
			}
			w.logger.Debugf("retry hook timer triggered for %s", w.unit.Tag().Id())
			w.retryHookTimerTriggered()

		case shutdown, ok := <-w.shutdownChannel:
			if !ok {
				return errors.New("shutdownChannel closed")
			}
			if shutdown {
				w.markShutdown()
			}
		}

		// Something changed.
		fire()
	}
}

// upgradeSeriesStatusChanged is called when the remote status of a series
// upgrade changes.
func (w *RemoteStateWatcher) upgradeSeriesStatusChanged() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	status, target, err := w.upgradeSeriesStatus()
	if errors.IsNotFound(err) {
		// There is no remote state so no upgrade is started.
		w.logger.Debugf("no upgrade series in progress, reinitializing local upgrade series state")
		w.current.UpgradeMachineStatus = model.UpgradeSeriesNotStarted
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	w.current.UpgradeMachineStatus = status
	w.current.UpgradeMachineTarget = target

	return nil
}

func (w *RemoteStateWatcher) upgradeSeriesStatus() (model.UpgradeSeriesStatus, string, error) {
	status, target, err := w.unit.UpgradeSeriesStatus()
	if err != nil {
		return "", "", errors.Trace(err)
	}

	graph := model.UpgradeSeriesGraph()
	if err := graph.Validate(); err != nil {
		return "", "", errors.Trace(err)
	}
	if !graph.ValidState(status) {
		return "", "", errors.NotValidf("upgrade series %q is", status)
	}
	return status, target, nil
}

// updateStatusChanged is called when the update status timer expires.
func (w *RemoteStateWatcher) updateStatusChanged() {
	w.mu.Lock()
	w.current.UpdateStatusVersion++
	w.mu.Unlock()
}

// commandsChanged is called when a command is enqueued.
func (w *RemoteStateWatcher) commandsChanged(id string) {
	w.mu.Lock()
	w.current.Commands = append(w.current.Commands, id)
	w.mu.Unlock()
}

// workloadEventsChanged is called when a container event is enqueued.
func (w *RemoteStateWatcher) workloadEventsChanged(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Ensure we don't add the same ID twice.
	for _, otherId := range w.current.WorkloadEvents {
		if otherId == id {
			return
		}
	}
	w.current.WorkloadEvents = append(w.current.WorkloadEvents, id)
}

// retryHookTimerTriggered is called when the retry hook timer expires.
func (w *RemoteStateWatcher) retryHookTimerTriggered() {
	w.mu.Lock()
	w.current.RetryHookVersion++
	w.mu.Unlock()
}

// unitChanged responds to changes in the unit.
func (w *RemoteStateWatcher) unitChanged() error {
	if err := w.unit.Refresh(); err != nil {
		return errors.Trace(err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current.Life = w.unit.Life()
	w.current.ResolvedMode = w.unit.Resolved()
	// It's ok to sync provider ID by watching unit rather than
	// cloud container because it will not change once pod created.
	w.current.ProviderID = w.unit.ProviderID()
	return nil
}

// applicationChanged responds to changes in the application.
func (w *RemoteStateWatcher) applicationChanged() error {
	if err := w.application.Refresh(); err != nil {
		return errors.Trace(err)
	}
	url, force, err := w.application.CharmURL()
	if err != nil {
		return errors.Trace(err)
	}
	required := false
	if w.canApplyCharmProfile {
		curl, err := charm.ParseURL(url)
		if err != nil {
			return errors.Trace(err)
		}
		ch, err := w.st.Charm(curl)
		if err != nil {
			return errors.Trace(err)
		}
		required, err = ch.LXDProfileRequired()
		if err != nil {
			return errors.Trace(err)
		}
	}
	ver, err := w.application.CharmModifiedVersion()
	if err != nil {
		return errors.Trace(err)
	}
	// CAAS sidecar charms will wait for the provider to restart/recreate
	// the unit before performing an upgrade.
	if w.sidecar && ver != w.enforcedCharmModifiedVersion {
		return nil
	}
	w.mu.Lock()
	w.current.CharmURL = url
	w.current.ForceCharmUpgrade = force
	w.current.CharmModifiedVersion = ver
	w.current.CharmProfileRequired = required
	w.mu.Unlock()
	return nil
}

// secretsChanged responds to changes in secrets.
func (w *RemoteStateWatcher) secretsChanged(secretURIs []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	info, err := w.secretsClient.GetConsumerSecretsRevisionInfo(w.unit.Tag().Id(), secretURIs)
	if err != nil {
		return errors.Trace(err)
	}
	w.logger.Debugf("got latest secret info: %#v", info)
	for _, uri := range secretURIs {
		if latest, ok := info[uri]; ok {
			w.current.ConsumedSecretInfo[uri] = latest
		} else {
			delete(w.current.ConsumedSecretInfo, uri)
			deleted := set.NewStrings(w.current.DeletedSecrets...)
			deleted.Add(uri)
			w.current.DeletedSecrets = deleted.SortedValues()
		}
	}
	w.logger.Debugf("deleted secrets: %v", w.current.DeletedSecrets)
	w.logger.Debugf("obsolete secrets: %v", w.current.ObsoleteSecretRevisions)
	return nil
}

func (w *RemoteStateWatcher) secretObsoleteRevisionsChanged(secretRevisions []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, revInfo := range secretRevisions {
		parts := strings.Split(revInfo, "/")
		uri := parts[0]
		if len(parts) < 2 {
			deleted := set.NewStrings(w.current.DeletedSecrets...)
			deleted.Add(uri)
			w.current.DeletedSecrets = deleted.SortedValues()
			continue
		}
		rev, err := strconv.Atoi(parts[1])
		if err != nil {
			return errors.NotValidf("secret revision %q for %q", parts[1], uri)
		}
		obsolete := set.NewInts(w.current.ObsoleteSecretRevisions[uri]...)
		obsolete.Add(rev)
		w.current.ObsoleteSecretRevisions[uri] = obsolete.SortedValues()
	}
	w.logger.Debugf("obsolete secret revisions: %v", w.current.ObsoleteSecretRevisions)
	w.logger.Debugf("deleted secrets: %v", w.current.DeletedSecrets)
	return nil
}

func (w *RemoteStateWatcher) instanceDataChanged() error {
	name, err := w.unit.LXDProfileName()
	if err != nil {
		return errors.Trace(err)
	}
	w.mu.Lock()
	w.current.LXDProfileName = name
	w.mu.Unlock()
	w.logger.Debugf("LXDProfileName changed to %q", name)
	return nil
}

func (w *RemoteStateWatcher) configHashChanged(value string) {
	w.mu.Lock()
	w.current.ConfigHash = value
	w.mu.Unlock()
}

func (w *RemoteStateWatcher) trustHashChanged(value string) {
	w.mu.Lock()
	w.current.TrustHash = value
	w.mu.Unlock()
}

func (w *RemoteStateWatcher) addressesHashChanged(value string) {
	w.mu.Lock()
	w.current.AddressesHash = value
	w.mu.Unlock()
}

func (w *RemoteStateWatcher) leaderSettingsChanged() error {
	w.mu.Lock()
	w.current.LeaderSettingsVersion++
	w.mu.Unlock()
	return nil
}

func (w *RemoteStateWatcher) leadershipChanged(isLeader bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.current.Leader = isLeader
	if w.secretRotateWatcher != nil {
		_ = worker.Stop(w.secretRotateWatcher)
	}
	w.secretRotateWatcher = nil
	w.rotateSecretsChanges = nil
	w.current.SecretRotations = nil

	if w.secretExpiryWatcher != nil {
		_ = worker.Stop(w.secretExpiryWatcher)
	}
	w.secretExpiryWatcher = nil
	w.expireSecretsChanges = nil
	w.current.ExpiredSecretRevisions = nil

	if w.obsoleteRevisionWatcher != nil {
		_ = worker.Stop(w.obsoleteRevisionWatcher)
	}
	w.obsoleteRevisionWatcher = nil
	w.obsoleteRevisionChanges = nil

	// Allow a generous buffer so a slow unit agent does not
	// block the upstream worker.
	w.rotateSecretsChanges = make(chan []string, 100)
	w.logger.Debugf("starting secrets rotation watcher")
	rotateWatcher, err := w.secretRotateWatcherFunc(w.unit.Tag(), isLeader, w.rotateSecretsChanges)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(rotateWatcher); err != nil {
		return errors.Trace(err)
	}
	w.secretRotateWatcher = rotateWatcher

	// Allow a generous buffer so a slow unit agent does not
	// block the upstream worker.
	w.expireSecretsChanges = make(chan []string, 100)
	w.logger.Debugf("starting secret revisions expiry watcher")
	expiryWatcher, err := w.secretExpiryWatcherFunc(w.unit.Tag(), isLeader, w.expireSecretsChanges)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(expiryWatcher); err != nil {
		return errors.Trace(err)
	}
	w.secretExpiryWatcher = expiryWatcher

	// Allow a generous buffer so a slow unit agent does not
	// block the upstream worker.
	w.obsoleteRevisionChanges = make(chan []string, 100)
	w.logger.Debugf("starting obsolete secret revisions watcher")
	owners := []names.Tag{w.unit.Tag()}
	if isLeader {
		appName, _ := names.UnitApplication(w.unit.Tag().Id())
		owners = append(owners, names.NewApplicationTag(appName))
	}
	obsoleteRevisionsWatcher, err := w.secretsClient.WatchObsolete(owners...)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(obsoleteRevisionsWatcher); err != nil {
		return errors.Trace(err)
	}
	w.obsoleteRevisionWatcher = obsoleteRevisionsWatcher
	w.obsoleteRevisionChanges = obsoleteRevisionsWatcher.Changes()

	return nil
}

// rotateSecretURIs adds the specified URLs to those that need
// to be rotated.
func (w *RemoteStateWatcher) rotateSecretURIs(uris []string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	pending := set.NewStrings(w.current.SecretRotations...)
	for _, uri := range uris {
		if !pending.Contains(uri) {
			pending.Add(uri)
			w.current.SecretRotations = append(w.current.SecretRotations, uri)
		}
	}
}

// expireSecretRevisions adds the specified secret revisions
// to those that need to be expired.
func (w *RemoteStateWatcher) expireSecretRevisions(revisions []string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	pending := set.NewStrings(w.current.ExpiredSecretRevisions...)
	for _, rev := range revisions {
		if !pending.Contains(rev) {
			pending.Add(rev)
			w.current.ExpiredSecretRevisions = append(w.current.ExpiredSecretRevisions, rev)
		}
	}
}

// relationsChanged responds to application relation changes.
func (w *RemoteStateWatcher) relationsChanged(keys []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// NOTE (stickupkid): Any return nil or early exit during the iteration of
	// the keys that is non-exhaustive can cause units (subordinates) to
	// commit suicide.

	for _, key := range keys {
		relationTag := names.NewRelationTag(key)
		rel, err := w.st.Relation(relationTag)
		if params.IsCodeNotFoundOrCodeUnauthorized(err) {
			// If it's actually gone, this unit cannot have entered
			// scope, and therefore never needs to know about it.
			if ruw, ok := w.relations[relationTag]; ok {
				_ = worker.Stop(ruw)
				delete(w.relations, relationTag)
				delete(w.current.Relations, ruw.relationId)
			}
		} else if err != nil {
			return errors.Trace(err)
		} else if relErr := w.ensureRelationUnits(rel); relErr != nil {
			return errors.Trace(relErr)
		}
	}
	return nil
}

func (w *RemoteStateWatcher) ensureRelationUnits(rel Relation) error {
	relationTag := rel.Tag()
	if _, ok := w.relations[relationTag]; ok {
		// We're already watching this one, so just update life/suspension status
		relationSnapshot := w.current.Relations[rel.Id()]
		relationSnapshot.Life = rel.Life()
		relationSnapshot.Suspended = rel.Suspended()
		w.current.Relations[rel.Id()] = relationSnapshot
		if rel.Suspended() {
			// Relation has been suspended, so stop the listeners here.
			// The relation itself is retained in the current relations
			// in the suspended state so that departed/broken hooks can run.
			if ruw, ok := w.relations[relationTag]; ok {
				err := worker.Stop(ruw)
				if err != nil {
					// This was always silently ignored, so it can't be
					// particularly useful, but avoid suppressing errors entirely.
					w.logger.Debugf("error stopping relation watcher for %s: %v", w.unit.Tag().Id(), err)
				}
				delete(w.relations, relationTag)
			}
		}
		return nil
	}
	// We weren't watching it already, but if the relation is suspended,
	// we don't need to start watching it.
	if rel.Suspended() {
		return nil
	}
	return errors.Trace(w.watchRelationUnits(rel))
}

// watchRelationUnits starts watching the relation units for the given
// relation, waits for its first event, and records the information in
// the current snapshot.
func (w *RemoteStateWatcher) watchRelationUnits(rel Relation) error {
	ruw, err := w.st.WatchRelationUnits(rel.Tag(), w.unit.Tag())
	// Deal with the race where Relation returned a valid, perhaps dying
	// relation, but by the time we ask to watch it, we get unauthorized
	// because it is no longer around.
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	// Because of the delay before handing off responsibility to
	// wrapRelationUnitsWatcher below, add to our own catacomb to
	// ensure errors get picked up if they happen.
	if err := w.catacomb.Add(ruw); err != nil {
		return errors.Trace(err)
	}
	relationSnapshot := RelationSnapshot{
		Life:               rel.Life(),
		Suspended:          rel.Suspended(),
		Members:            make(map[string]int64),
		ApplicationMembers: make(map[string]int64),
	}
	// Handle the first change to populate the Members map.
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case change, ok := <-ruw.Changes():
		if !ok {
			return errors.New("relation units watcher closed")
		}
		for unit, settings := range change.Changed {
			relationSnapshot.Members[unit] = settings.Version
		}
		for app, settingsVersion := range change.AppChanged {
			relationSnapshot.ApplicationMembers[app] = settingsVersion
		}
	}
	// Wrap the Changes() with the relationId so we can process all changes
	// via the same channel.
	innerRUW, err := wrapRelationUnitsWatcher(rel.Id(), ruw, w.relationUnitsChanges)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(innerRUW); err != nil {
		return errors.Trace(err)
	}
	w.current.Relations[rel.Id()] = relationSnapshot
	w.relations[rel.Tag()] = innerRUW
	return nil
}

// relationUnitsChanged responds to relation units changes.
func (w *RemoteStateWatcher) relationUnitsChanged(change relationUnitsChange) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	snapshot, ok := w.current.Relations[change.relationId]
	if !ok {
		return nil
	}
	for unit, settings := range change.Changed {
		snapshot.Members[unit] = settings.Version
	}
	for app, settingsVersion := range change.AppChanged {
		snapshot.ApplicationMembers[app] = settingsVersion
	}
	for _, unit := range change.Departed {
		delete(snapshot.Members, unit)
	}
	return nil
}

// storageAttachmentChanged responds to storage attachment changes.
func (w *RemoteStateWatcher) storageAttachmentChanged(change storageAttachmentChange) {
	w.mu.Lock()
	w.current.Storage[change.Tag] = change.Snapshot
	w.mu.Unlock()
}

func (w *RemoteStateWatcher) actionsChanged(actions []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, action := range actions {
		// If we already have the action, signal a change.
		if r, ok := w.current.ActionChanged[action]; ok {
			w.current.ActionChanged[action] = r + 1
		} else {
			w.current.ActionsPending = append(w.current.ActionsPending, action)
			w.current.ActionChanged[action] = 0
		}
	}
}

func (w *RemoteStateWatcher) containerRunningStatus(runningStatus ContainerRunningStatus) {
	w.mu.Lock()
	w.current.ActionsBlocked = !runningStatus.Running
	w.current.ContainerRunningStatus = &runningStatus
	w.mu.Unlock()
}

// storageChanged responds to unit storage changes.
func (w *RemoteStateWatcher) storageChanged(keys []string) error {
	tags := make([]names.StorageTag, len(keys))
	for i, key := range keys {
		tags[i] = names.NewStorageTag(key)
	}
	ids := make([]params.StorageAttachmentId, len(keys))
	for i, tag := range tags {
		ids[i] = params.StorageAttachmentId{
			StorageTag: tag.String(),
			UnitTag:    w.unit.Tag().String(),
		}
	}
	results, err := w.st.StorageAttachmentLife(ids)
	if err != nil {
		return errors.Trace(err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for i, result := range results {
		tag := tags[i]
		if result.Error == nil {
			if storageSnapshot, ok := w.current.Storage[tag]; ok {
				// We've previously started a watcher for this storage
				// attachment, so all we needed to do was update the
				// lifecycle state.
				storageSnapshot.Life = result.Life
				w.current.Storage[tag] = storageSnapshot
				continue
			}
			// We haven't seen this storage attachment before, so start
			// a watcher now; add it to our catacomb in case of mishap;
			// and wait for the initial event.
			saw, err := w.st.WatchStorageAttachment(tag, w.unit.Tag())
			if err != nil {
				return errors.Annotate(err, "watching storage attachment")
			}
			if err := w.catacomb.Add(saw); err != nil {
				return errors.Trace(err)
			}
			if err := w.watchStorageAttachment(tag, result.Life, saw); err != nil {
				return errors.Trace(err)
			}
		} else if params.IsCodeNotFound(result.Error) {
			if watcher, ok := w.storageAttachmentWatchers[tag]; ok {
				// already under catacomb management, any error tracked already
				_ = worker.Stop(watcher)
				delete(w.storageAttachmentWatchers, tag)
			}
			delete(w.current.Storage, tag)
		} else {
			return errors.Annotatef(
				result.Error, "getting life of %s attachment for %s",
				names.ReadableString(tag), w.unit.Tag().Id(),
			)
		}
	}
	return nil
}

// watchStorageAttachment starts watching the storage attachment with
// the specified storage tag, waits for its first event, and records
// the information in the current snapshot.
func (w *RemoteStateWatcher) watchStorageAttachment(
	tag names.StorageTag,
	life life.Value,
	saw watcher.NotifyWatcher,
) error {
	var storageSnapshot StorageSnapshot
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case _, ok := <-saw.Changes():
		if !ok {
			return errors.Errorf("storage attachment watcher closed for %s", w.unit.Tag().Id())
		}
		var err error
		storageSnapshot, err = getStorageSnapshot(w.st, tag, w.unit.Tag())
		if errors.Is(err, errors.NotProvisioned) {
			// If the storage is unprovisioned, we still want to
			// record the attachment, but we'll mark it as
			// unattached. This allows the uniter to wait for
			// pending storage attachments to be provisioned.
			storageSnapshot = StorageSnapshot{Life: life}
		} else if err != nil {
			return errors.Annotatef(err, "processing initial storage attachment change for %s", w.unit.Tag().Id())
		}
	}
	innerSAW, err := newStorageAttachmentWatcher(
		w.st, saw, w.unit.Tag(), tag, w.storageAttachmentChanges,
	)
	if err != nil {
		return errors.Trace(err)
	}
	w.current.Storage[tag] = storageSnapshot
	w.storageAttachmentWatchers[tag] = innerSAW
	return nil
}

// markShutdown is called when Shutdown is called on remote state.
func (w *RemoteStateWatcher) markShutdown() {
	w.mu.Lock()
	w.current.Shutdown = true
	w.mu.Unlock()
}
