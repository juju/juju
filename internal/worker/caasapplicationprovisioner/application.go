// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type appNotifyWorker interface {
	worker.Worker
	Notify()
}

type appWorker struct {
	catacomb   catacomb.Catacomb
	facade     CAASProvisionerFacade
	broker     CAASBroker
	clock      clock.Clock
	logger     Logger
	unitFacade CAASUnitProvisionerFacade
	ops        ApplicationOps

	name        string
	modelTag    names.ModelTag
	changes     chan struct{}
	password    string
	lastApplied caas.ApplicationConfig
	life        life.Value
	statusOnly  bool

	engineReportRequest chan chan<- map[string]any
}

type AppWorkerConfig struct {
	Name       string
	Facade     CAASProvisionerFacade
	Broker     CAASBroker
	ModelTag   names.ModelTag
	Clock      clock.Clock
	Logger     Logger
	UnitFacade CAASUnitProvisionerFacade
	Ops        ApplicationOps
	StatusOnly bool
}

type appLoopState struct {
	app                 caas.Application
	appLife             life.Value
	appProvisionChanges watcher.NotifyChannel
	appChanges          watcher.NotifyChannel
	replicaChanges      watcher.NotifyChannel

	done    bool
	ready   bool
	initial bool

	scaleChan              <-chan time.Time
	scaleTries             int
	trustChan              <-chan time.Time
	trustTries             int
	reconcileDeadChan      <-chan time.Time
	stateAppChangedChan    <-chan time.Time
	storageConstraintsChan <-chan time.Time
}

const tryAgain errors.ConstError = "try again"

type NewAppWorkerFunc func(AppWorkerConfig) func() (worker.Worker, error)

func NewAppWorker(config AppWorkerConfig) func() (worker.Worker, error) {
	ops := config.Ops
	if ops == nil {
		ops = &applicationOps{}
	}
	return func() (worker.Worker, error) {
		changes := make(chan struct{}, 1)
		changes <- struct{}{}
		a := &appWorker{
			name:                config.Name,
			facade:              config.Facade,
			broker:              config.Broker,
			modelTag:            config.ModelTag,
			clock:               config.Clock,
			logger:              config.Logger,
			changes:             changes,
			unitFacade:          config.UnitFacade,
			ops:                 ops,
			statusOnly:          config.StatusOnly,
			engineReportRequest: make(chan chan<- map[string]any),
		}
		err := catacomb.Invoke(catacomb.Plan{
			Site: &a.catacomb,
			Work: a.loop,
		})
		return a, err
	}
}

func (a *appWorker) Notify() {
	select {
	case a.changes <- struct{}{}:
	case <-a.catacomb.Dying():
	}
}

func (a *appWorker) Kill() {
	a.catacomb.Kill(nil)
}

func (a *appWorker) Wait() error {
	return a.catacomb.Wait()
}

func (a *appWorker) loop() error {
	// TODO(sidecar): support more than statefulset
	state := &appLoopState{
		app:     a.broker.Application(a.name, caas.DeploymentStateful),
		initial: true,
	}

	// If the application no longer exists, return immediately. If it's in
	// Dead state, ensure it's deleted and terminated.
	appLife, err := a.facade.Life(a.name)
	if errors.Is(err, errors.NotFound) {
		a.logger.Debugf("application %q no longer exists", a.name)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "fetching life status for application %q", a.name)
	}
	a.life = appLife
	if appLife == life.Dead {
		if !a.statusOnly {
			err = a.ops.AppDying(a.name, state.app, a.life, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", a.name)
			}
			err = a.ops.AppDead(a.name, state.app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", a.name)
			}
		}
		return nil
	}

	if appLife == life.Alive && !a.statusOnly {
		// Ensure the charm is upgraded to a v2 charm (or wait for that).
		shouldExit, err := a.ops.VerifyCharmUpgraded(a.name, a.facade, &a.catacomb, a.logger)
		if err != nil {
			return errors.Trace(err)
		}
		if shouldExit {
			return nil
		}

		err = a.ops.UpgradePodSpec(a.name, a.broker, a.clock, &a.catacomb, a.logger)
		if err != nil {
			return errors.Trace(err)
		}

		// Update the password once per worker start to avoid it changing too frequently.
		a.password, err = utils.RandomPassword()
		if err != nil {
			return errors.Trace(err)
		}
		err = a.facade.SetPassword(a.name, a.password)
		if err != nil {
			return errors.Annotate(err, "failed to set application api passwords")
		}

		ps, err := a.facade.ProvisioningState(a.name)
		if err != nil {
			return errors.Annotatef(err, "getting provision state for %q", a.name)
		}
		if ps == nil {
			ps = &params.CAASApplicationProvisioningState{}
		}

		// We have to resume the current operation (if one exists) on worker
		// restart.
		if ps.CurrentOperation == application.StorageUpdateOperation {
			a.logger.Debugf("app %q resuming storage update operation", a.name)
			err := a.ops.EnsureStorage(a.name, state.app, &a.lastApplied, a.password,
				a.facade, a.clock, a.logger)
			if err != nil {
				return errors.Annotatef(err, "ensuring storage for %q", a.name)
			}
		} else if ps.CurrentOperation == application.ScaleOperation {
			a.logger.Debugf("app %q resuming scale operation", a.name)
			err := a.ops.EnsureScale(a.name, state.app, a.life, a.facade, a.unitFacade, a.logger)
			if err != nil && !errors.Is(err, tryAgain) && !errors.Is(err, errors.NotFound) {
				return errors.Annotatef(err, "scaling app %q", a.name)
			}
		}
	}

	var lastReportedStatus map[string]status.StatusInfo

	appScaleWatcher, err := a.unitFacade.WatchApplicationScale(a.name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q scale watcher", a.name)
	}
	if err := a.catacomb.Add(appScaleWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q scale changes", a.name)
	}

	appTrustWatcher, err := a.unitFacade.WatchApplicationTrustHash(a.name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q trust watcher", a.name)
	}
	if err := a.catacomb.Add(appTrustWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q trust changes", a.name)
	}

	var appUnitsWatcher watcher.StringsWatcher
	appUnitsWatcher, err = a.facade.WatchUnits(a.name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q units watcher", a.name)
	}
	if err := a.catacomb.Add(appUnitsWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q units changes", a.name)
	}

	var storageConstraintsWatcher watcher.NotifyWatcher
	storageConstraintsWatcher, err = a.facade.WatchStorageConstraints(a.name)
	if err != nil {
		return errors.Annotatef(err,
			"creating application %q storage constraints watcher", a.name)
	}
	if err := a.catacomb.Add(storageConstraintsWatcher); err != nil {
		return errors.Annotatef(err,
			"failed to watch for application %q storage constraints changes", a.name)
	}

	const (
		maxRetries = 20
		retryDelay = 3 * time.Second
	)

	refreshTimer := a.clock.NewTimer(10 * time.Second)
	defer refreshTimer.Stop()
	for {
		shouldRefresh := true
		select {
		case _, ok := <-appScaleWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q scale watcher closed channel", a.name)
			}
			if state.scaleChan == nil {
				state.scaleTries = 0
				state.scaleChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-state.scaleChan:
			if a.statusOnly {
				state.scaleChan = nil
				break
			}
			if !state.ready {
				state.scaleChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
			err := a.ops.EnsureScale(a.name, state.app, a.life, a.facade, a.unitFacade, a.logger)
			if errors.Is(err, errors.NotFound) {
				if state.scaleTries >= maxRetries {
					return errors.Annotatef(err, "more than %d retries ensuring scale", maxRetries)
				}
				state.scaleTries++
				state.scaleChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if errors.Is(err, tryAgain) {
				state.scaleChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return errors.Trace(err)
			} else {
				state.scaleChan = nil
			}
		case _, ok := <-appTrustWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q trust watcher closed channel", a.name)
			}
			if state.trustChan == nil {
				state.trustTries = 0
				state.trustChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-state.trustChan:
			if a.statusOnly {
				state.trustChan = nil
				break
			}
			if !state.ready {
				state.trustChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
			err := a.ops.EnsureTrust(a.name, state.app, a.unitFacade, a.logger)
			if errors.Is(err, errors.NotFound) {
				if state.trustTries >= maxRetries {
					return errors.Annotatef(err, "more than %d retries ensuring trust", maxRetries)
				}
				state.trustTries++
				state.trustChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return errors.Trace(err)
			} else {
				state.trustChan = nil
			}
		case _, ok := <-appUnitsWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q units watcher closed channel", a.name)
			}
			if state.reconcileDeadChan == nil {
				state.reconcileDeadChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-state.reconcileDeadChan:
			if a.statusOnly {
				state.reconcileDeadChan = nil
				break
			}
			err := a.ops.ReconcileDeadUnitScale(a.name, state.app, a.life, a.facade, a.logger)
			if errors.Is(err, errors.NotFound) {
				state.reconcileDeadChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if errors.Is(err, tryAgain) {
				state.reconcileDeadChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return fmt.Errorf("reconciling dead unit scale: %w", err)
			} else {
				state.reconcileDeadChan = nil
			}
		case <-a.catacomb.Dying():
			return a.catacomb.ErrDying()
		case <-state.appProvisionChanges:
			if state.stateAppChangedChan == nil {
				state.stateAppChangedChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-a.changes:
			if state.stateAppChangedChan == nil {
				state.stateAppChangedChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-state.stateAppChangedChan:
			// Respond to life changes (Notify called by parent worker).
			err = a.handleLifeChange(state)
			if errors.Is(err, tryAgain) {
				state.stateAppChangedChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return errors.Trace(err)
			} else {
				state.stateAppChangedChan = nil
			}
		case <-state.appChanges:
			// Respond to changes in provider application.
			lastReportedStatus, err = a.ops.UpdateState(a.name, state.app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		case <-state.replicaChanges:
			// Respond to changes in replicas of the application.
			lastReportedStatus, err = a.ops.UpdateState(a.name, state.app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-storageConstraintsWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q storage constraints watcher closed channel", a.name)
			}
			if state.storageConstraintsChan == nil {
				state.storageConstraintsChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-state.storageConstraintsChan:
			if !state.ready {
				state.storageConstraintsChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
			err = a.handleStorageChange(state)
			if errors.Is(err, tryAgain) || errors.Is(err, errors.NotProvisioned) {
				state.storageConstraintsChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return errors.Trace(err)
			}
		case <-a.clock.After(10 * time.Second):
		case <-refreshTimer.Chan():
			// Force refresh of application status.
		case reportChan := <-a.engineReportRequest:
			// Respond to engine reports.
			var reportErrors []string
			ps, err := a.facade.ProvisioningState(a.name)
			if err != nil {
				reportErrors = append(reportErrors, err.Error())
			}
			if ps == nil {
				ps = &params.CAASApplicationProvisioningState{}
			}
			report := map[string]any{
				"application-name":  a.name,
				"status-only":       a.statusOnly,
				"application-life":  a.life,
				"scale-target":      ps.ScaleTarget,
				"current-operation": ps.CurrentOperation,
				"report-error":      reportErrors,
			}
			select {
			case reportChan <- report:
			case <-a.catacomb.Dying():
				return a.catacomb.ErrDying()
			}
			shouldRefresh = false
		}
		if state.done {
			return nil
		}
		if shouldRefresh {
			if err = a.ops.RefreshApplicationStatus(a.name, state.app, appLife, a.facade, a.logger); err != nil {
				return errors.Annotatef(err, "refreshing application status for %q", a.name)
			}
		}
	}
}

// handleLifeChange processes transitions in the application's lifecycle.
// It retrieves the current lifecycle status (Alive, Dying, or Dead) and
// reconciles the provider's actual state to match. For newly alive
// applications, it initializes resource watchers (provisioning, application,
// and replicas) and triggers [ApplicationOps.AppAlive]. For dying or dead
// applications, it invokes cleanup operations ([ApplicationOps.AppDying] and
// [ApplicationOps.AppDead]).
// It also updates the provided appLoopState with the current readiness and completion
// status, while respecting status-only worker configurations.
func (a *appWorker) handleLifeChange(state *appLoopState) error {
	appLife, err := a.facade.Life(a.name)
	if errors.Is(err, errors.NotFound) {
		appLife = life.Dead
	} else if err != nil {
		return errors.Trace(err)
	}
	a.life = appLife
	state.appLife = appLife

	if state.initial {
		state.initial = false
		ps, err := a.facade.ProvisioningState(a.name)
		if err != nil {
			return errors.Trace(err)
		}
		if ps != nil && ps.CurrentOperation == application.ScaleOperation {
			if a.statusOnly {
				// Clear provisioning state for status only app.
				err = a.facade.SetProvisioningState(a.name, params.CAASApplicationProvisioningState{})
				if err != nil {
					return errors.Trace(err)
				}
			} else {
				state.scaleChan = a.clock.After(0)
				state.reconcileDeadChan = a.clock.After(0)
			}
		}
	}

	switch state.appLife {
	case life.Alive:
		if state.appProvisionChanges == nil {
			appProvisionWatcher, err := a.facade.WatchProvisioningInfo(a.name)
			if err != nil {
				return errors.Annotatef(err, "failed to watch facade for changes to application provisioning %q", a.name)
			}
			if err := a.catacomb.Add(appProvisionWatcher); err != nil {
				return errors.Trace(err)
			}
			state.appProvisionChanges = appProvisionWatcher.Changes()
		}
		if !a.statusOnly {
			err := a.ops.AppAlive(a.name, state.app, a.password, &a.lastApplied, a.facade, a.clock, a.logger)
			if errors.Is(err, errors.NotProvisioned) {
				// State not ready for this application to be provisioned yet.
				// Usually because the charm has not yet been downloaded.
				break
			} else if err != nil {
				return errors.Trace(err)
			}
		}
		if state.appChanges == nil {
			appWatcher, err := state.app.Watch()
			if err != nil {
				return errors.Annotatef(err, "failed to watch for changes to application %q", a.name)
			}
			if err := a.catacomb.Add(appWatcher); err != nil {
				return errors.Trace(err)
			}
			state.appChanges = appWatcher.Changes()
		}
		if state.replicaChanges == nil {
			replicaWatcher, err := state.app.WatchReplicas()
			if err != nil {
				return errors.Annotatef(err, "failed to watch for changes to replicas %q", a.name)
			}
			if err := a.catacomb.Add(replicaWatcher); err != nil {
				return errors.Trace(err)
			}
			state.replicaChanges = replicaWatcher.Changes()
		}
		a.logger.Debugf("application %q is ready", a.name)
		state.ready = true

	case life.Dying:
		if !a.statusOnly {
			err := a.ops.AppDying(a.name, state.app, a.life, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		}
		state.ready = false

	case life.Dead:
		if !a.statusOnly {
			err := a.ops.AppDying(a.name, state.app, a.life, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
			err = a.ops.AppDead(a.name, state.app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		}
		state.done = true
		state.ready = false
		return nil

	default:
		return errors.NotImplementedf("unknown life %q", a.life)
	}
	return nil
}

// handleStorageChange synchronizes the underlying provider's storage allocations
// with the application's current storage constraints. It evaluates the application's
// lifecycle status and only applies storage changes via [ApplicationOps.EnsureStorage]
// if the application is currently Alive. If the worker is operating in a status-only
// mode, or if the application is no longer active, it safely bypasses the update
// and clears the pending storage constraints channel in the appLoopState.
func (a *appWorker) handleStorageChange(state *appLoopState) error {
	if a.statusOnly {
		state.storageConstraintsChan = nil
		return nil
	}

	appLife, err := a.facade.Life(a.name)
	if errors.Is(err, errors.NotFound) {
		state.storageConstraintsChan = nil
		a.logger.Debugf("application %q no longer exists, skipping storage update", a.name)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "fetching life status for application %q", a.name)
	}
	a.life = appLife
	state.appLife = appLife

	switch appLife {
	case life.Alive:
		err := a.ops.EnsureStorage(a.name, state.app, &a.lastApplied, a.password,
			a.facade, a.clock, a.logger)
		if err != nil {
			return errors.Trace(err)
		}
	default:
		a.logger.Debugf("application %q is not alive (%s), skipping storage update", a.name, appLife)
	}
	state.storageConstraintsChan = nil

	return nil
}

// Report returns a report about this application provisioner.
func (a *appWorker) Report() map[string]any {
	reportChan := make(chan map[string]any)
	select {
	case a.engineReportRequest <- reportChan:
	case <-a.catacomb.Dying():
		return nil
	}
	select {
	case report := <-reportChan:
		return report
	case <-a.catacomb.Dying():
		return nil
	}
}
