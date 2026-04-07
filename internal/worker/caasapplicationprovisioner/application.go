// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/caas"
<<<<<<< HEAD
	coreapplication "github.com/juju/juju/core/application"
=======
	"github.com/juju/juju/core/application"
>>>>>>> 3.6
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/password"
)

type appNotifyWorker interface {
	worker.Worker
	Notify()
}

type appWorker struct {
	catacomb catacomb.Catacomb

<<<<<<< HEAD
	agentPasswordService       AgentPasswordService
	applicationService         ApplicationService
	statusService              StatusService
	storageProvisioningService StorageProvisioningService
	resourceOpenerGetter       ResourceOpenerGetter

	facade CAASProvisionerFacade
	broker CAASBroker
	clock  clock.Clock
	logger logger.Logger
	ops    ApplicationOps

	appUUID          coreapplication.UUID
	changes          chan struct{}
	password         string
	lastApplied      caas.ApplicationConfig
	provisioningInfo *ProvisioningInfo
	life             life.Value
=======
	name        string
	modelTag    names.ModelTag
	changes     chan struct{}
	password    string
	lastApplied caas.ApplicationConfig
	statusOnly  bool
>>>>>>> 3.6

	engineReportRequest chan chan<- map[string]any
}

type AppWorkerConfig struct {
	AppID coreapplication.UUID

	AgentPasswordService       AgentPasswordService
	ApplicationService         ApplicationService
	StatusService              StatusService
	StorageProvisioningService StorageProvisioningService
	ResourceOpenerGetter       ResourceOpenerGetter

	Ops    ApplicationOps
	Broker CAASBroker
	Clock  clock.Clock
	Logger logger.Logger

	// TODO: remove these
	Facade CAASProvisionerFacade
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

type NewAppWorkerFunc func(AppWorkerConfig) func(ctx context.Context) (worker.Worker, error)

func NewAppWorker(config AppWorkerConfig) func(ctx context.Context) (worker.Worker, error) {
	ops := config.Ops
	if ops == nil {
		ops = &applicationOps{}
	}

	return func(ctx context.Context) (worker.Worker, error) {
		changes := make(chan struct{}, 1)
		changes <- struct{}{}
		a := &appWorker{
			agentPasswordService:       config.AgentPasswordService,
			applicationService:         config.ApplicationService,
			statusService:              config.StatusService,
			storageProvisioningService: config.StorageProvisioningService,
			resourceOpenerGetter:       config.ResourceOpenerGetter,
			appUUID:                    config.AppID,
			facade:                     config.Facade,
			broker:                     config.Broker,
			clock:                      config.Clock,
			logger:                     config.Logger,
			changes:                    changes,
			ops:                        ops,
			engineReportRequest:        make(chan chan<- map[string]any),
		}
		err := catacomb.Invoke(catacomb.Plan{
			Name: "caas-application-provisioner",
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
	ctx, cancel := a.scopedContext()
	defer cancel()

	// TODO: eliminate name at this level, it should be only in provisioning info
	// when creating resources on the k8s broker.
	name, err := a.applicationService.GetApplicationName(ctx, a.appUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		a.logger.Debugf(ctx, "application %q no longer exists", a.appUUID)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "fetching info for application %q", a.appUUID)
	}

	// If the application is the Juju controller, only provide updates on the
	// status of the application.
	statusOnly, err := a.applicationService.IsControllerApplication(ctx, a.appUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		a.logger.Debugf(ctx, "application %q no longer exists", a.appUUID)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "fetching info for application %q", a.appUUID)
	}

	// TODO(sidecar): support more than statefulset
<<<<<<< HEAD
	app := a.broker.Application(name, caas.DeploymentStateful)
=======
	state := &appLoopState{
		app:     a.broker.Application(a.name, caas.DeploymentStateful),
		initial: true,
	}
>>>>>>> 3.6

	// If the application no longer exists, return immediately. If it's in
	// Dead state, ensure it's deleted and terminated.
	appLife, err := a.applicationService.GetApplicationLife(ctx, a.appUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		a.logger.Debugf(ctx, "application %q no longer exists", name)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "fetching life status for application %q", name)
	}
	state.appLife = appLife
	if appLife == life.Dead {
<<<<<<< HEAD
		if !statusOnly {
			err = a.ops.AppDying(ctx, name, a.appUUID, app, a.life, a.facade,
				a.applicationService, a.statusService, a.logger)
=======
		if !a.statusOnly {
			err = a.ops.AppDying(a.name, state.app, state.appLife, a.facade, a.unitFacade, a.logger)
>>>>>>> 3.6
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", name)
			}
<<<<<<< HEAD
			err = a.ops.AppDead(ctx, name, a.appUUID, app, a.broker,
				a.applicationService, a.statusService, a.clock, a.logger)
=======
			err = a.ops.AppDead(a.name, state.app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
>>>>>>> 3.6
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", name)
			}
		}
		return nil
	}

	if appLife == life.Alive && !statusOnly {
		// Update the password once per worker start to avoid it changing too frequently.
		a.password, err = password.RandomPassword()
		if err != nil {
			return errors.Trace(err)
		}
		err = a.agentPasswordService.SetApplicationPassword(ctx, a.appUUID, a.password)
		if err != nil {
			return errors.Annotate(err, "failed to set application api password")
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
			err := a.ops.EnsureStorage(a.name, state.app, a.password,
				a.facade, a.clock, a.logger)
			if err != nil {
				return errors.Annotatef(err, "ensuring storage for %q", a.name)
			}
		} else if ps.CurrentOperation == application.ScaleOperation {
			a.logger.Debugf("app %q resuming scale operation", a.name)
			err := a.ops.EnsureScale(a.name, state.app, state.appLife, a.facade, a.unitFacade, a.logger)
			if err != nil && !errors.Is(err, tryAgain) && !errors.Is(err, errors.NotFound) {
				return errors.Annotatef(err, "scaling app %q", a.name)
			}
		}
	}

<<<<<<< HEAD
	var appChanges watcher.NotifyChannel
	var appProvisionChanges watcher.NotifyChannel
	var replicaChanges watcher.NotifyChannel
	var lastReportedStatus UpdateStatusState
=======
	var lastReportedStatus map[string]status.StatusInfo
>>>>>>> 3.6

	appScaleWatcher, err := a.applicationService.WatchApplicationScale(ctx, name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q scale watcher", name)
	}
	if err := a.catacomb.Add(appScaleWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q scale changes", name)
	}

	appSettingsWatcher, err := a.applicationService.WatchApplicationSettings(ctx, name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q trust watcher", name)
	}
	if err := a.catacomb.Add(appSettingsWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q trust changes", name)
	}

	appUnitsWatcher, err := a.applicationService.WatchApplicationUnitLife(ctx, name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q units life watcher", name)
	}
	if err := a.catacomb.Add(appUnitsWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q units life changes", name)
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

<<<<<<< HEAD
	handleChange := func() error {
		appLife, err := a.applicationService.GetApplicationLife(ctx, a.appUUID)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			appLife = life.Dead
		} else if err != nil {
			return errors.Trace(err)
		}
		a.life = appLife

		if initial {
			initial = false
			ps, err := a.applicationService.GetApplicationScalingState(ctx, name)
			if err != nil {
				return errors.Trace(err)
			}
			if ps.Scaling {
				if statusOnly {
					// Clear provisioning state for status only app.
					err = a.applicationService.SetApplicationScalingState(ctx, name, 0, false)
					if err != nil {
						return errors.Trace(err)
					}
				} else {
					scaleChan = a.clock.After(0)
					reconcileDeadChan = a.clock.After(0)
				}
			}
		}
		switch appLife {
		case life.Alive:
			if appProvisionChanges == nil {
				appProvisionWatcher, err := a.facade.WatchProvisioningInfo(ctx, name)
				if err != nil {
					return errors.Annotatef(err, "failed to watch facade for changes to application provisioning %q", name)
				}
				if err := a.catacomb.Add(appProvisionWatcher); err != nil {
					return errors.Trace(err)
				}
				appProvisionChanges = appProvisionWatcher.Changes()
			}
			if !statusOnly {
				a.provisioningInfo, err = a.ops.ProvisioningInfo(ctx, name,
					a.appUUID, a.facade, a.applicationService,
					a.storageProvisioningService, a.resourceOpenerGetter,
					a.provisioningInfo, a.logger)
				if errors.Is(err, errors.NotProvisioned) {
					a.logger.Debugf(ctx, "application %q is not provisioned", name)
					// State not ready for this application to be provisioned yet.
					// Usually because the charm has not yet been downloaded.
					return tryAgain
				} else if err != nil {
					return errors.Annotatef(err, "failed to get provisioning info for %q", name)
				}
				err = a.ops.AppAlive(ctx, name, a.appUUID, app, a.password,
					&a.lastApplied, a.provisioningInfo, a.statusService,
					a.clock, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
			}
			if appChanges == nil {
				appWatcher, err := app.Watch(ctx)
				if err != nil {
					return errors.Annotatef(err, "failed to watch for changes to application %q", name)
				}
				if err := a.catacomb.Add(appWatcher); err != nil {
					return errors.Trace(err)
				}
				appChanges = appWatcher.Changes()
			}
			if replicaChanges == nil {
				replicaWatcher, err := app.WatchReplicas()
				if err != nil {
					return errors.Annotatef(err, "failed to watch for changes to replicas %q", name)
				}
				if err := a.catacomb.Add(replicaWatcher); err != nil {
					return errors.Trace(err)
				}
				replicaChanges = replicaWatcher.Changes()
			}
			a.logger.Debugf(ctx, "application %q is ready", name)
			ready = true
		case life.Dying:
			if !statusOnly {
				err = a.ops.AppDying(ctx, name, a.appUUID, app, a.life,
					a.facade, a.applicationService, a.statusService, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
			}
			ready = false
		case life.Dead:
			if !statusOnly {
				err = a.ops.AppDying(ctx, name, a.appUUID, app, a.life,
					a.facade, a.applicationService, a.statusService, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
				err = a.ops.AppDead(ctx, name, a.appUUID, app, a.broker,
					a.applicationService, a.statusService, a.clock, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
			}
			done = true
			ready = false
			return nil
		default:
			return errors.NotImplementedf("unknown life %q", a.life)
		}
		return nil
	}

=======
>>>>>>> 3.6
	refreshTimer := a.clock.NewTimer(10 * time.Second)
	defer refreshTimer.Stop()
	for {
		shouldRefresh := true
		select {
		case _, ok := <-appScaleWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q scale watcher closed channel", name)
			}
			if state.scaleChan == nil {
				state.scaleTries = 0
				state.scaleChan = a.clock.After(0)
			}
			shouldRefresh = false
<<<<<<< HEAD
		case <-scaleChan:
			if statusOnly {
				scaleChan = nil
=======
		case <-state.scaleChan:
			if a.statusOnly {
				state.scaleChan = nil
>>>>>>> 3.6
				break
			}
			if !state.ready {
				state.scaleChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
<<<<<<< HEAD
			err := a.ops.EnsureScale(ctx, name, a.appUUID, app, a.life, a.facade,
				a.applicationService, a.logger)
=======
			err := a.ops.EnsureScale(a.name, state.app, state.appLife, a.facade, a.unitFacade, a.logger)
>>>>>>> 3.6
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
		case _, ok := <-appSettingsWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q trust watcher closed channel", name)
			}
			if state.trustChan == nil {
				state.trustTries = 0
				state.trustChan = a.clock.After(0)
			}
			shouldRefresh = false
<<<<<<< HEAD
		case <-trustChan:
			if statusOnly {
				trustChan = nil
=======
		case <-state.trustChan:
			if a.statusOnly {
				state.trustChan = nil
>>>>>>> 3.6
				break
			}
			if !state.ready {
				state.trustChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
<<<<<<< HEAD
			err := a.ops.EnsureTrust(ctx, name, app, a.applicationService, a.logger)
=======
			err := a.ops.EnsureTrust(a.name, state.app, a.unitFacade, a.logger)
>>>>>>> 3.6
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
				return fmt.Errorf("application %q units watcher closed channel", name)
			}
			if state.reconcileDeadChan == nil {
				state.reconcileDeadChan = a.clock.After(0)
			}
			shouldRefresh = false
<<<<<<< HEAD
		case <-reconcileDeadChan:
			if statusOnly {
				reconcileDeadChan = nil
				break
			}
			err := a.ops.ReconcileDeadUnitScale(ctx, name, a.appUUID, app,
				a.facade, a.applicationService, a.logger)
=======
		case <-state.reconcileDeadChan:
			if a.statusOnly {
				state.reconcileDeadChan = nil
				break
			}
			err := a.ops.ReconcileDeadUnitScale(a.name, state.app, state.appLife, a.facade, a.logger)
>>>>>>> 3.6
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
<<<<<<< HEAD
			lastReportedStatus, err = a.ops.UpdateState(
				ctx, name, a.appUUID, app, lastReportedStatus,
				a.broker, a.applicationService,
				a.statusService, a.clock, a.logger)
=======
			lastReportedStatus, err = a.ops.UpdateState(a.name, state.app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
>>>>>>> 3.6
			if err != nil {
				return errors.Trace(err)
			}
		case <-state.replicaChanges:
			// Respond to changes in replicas of the application.
<<<<<<< HEAD
			lastReportedStatus, err = a.ops.UpdateState(
				ctx, name, a.appUUID, app, lastReportedStatus,
				a.broker, a.applicationService,
				a.statusService, a.clock, a.logger)
=======
			lastReportedStatus, err = a.ops.UpdateState(a.name, state.app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
>>>>>>> 3.6
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
		case <-refreshTimer.Chan():
			// Force refresh of application status.
		case reportChan := <-a.engineReportRequest:
			// Respond to engine reports.
			var reportErrors []string
			ps, err := a.applicationService.GetApplicationScalingState(ctx, name)
			if err != nil {
				reportErrors = append(reportErrors, err.Error())
			}
			report := map[string]any{
<<<<<<< HEAD
				"application-uuid": a.appUUID,
				"application-name": name,
				"status-only":      statusOnly,
				"application-life": a.life,
				"scale-target":     ps.ScaleTarget,
				"scaling":          ps.Scaling,
				"report-error":     reportErrors,
=======
				"application-name":  a.name,
				"status-only":       a.statusOnly,
				"application-life":  state.appLife,
				"scale-target":      ps.ScaleTarget,
				"current-operation": ps.CurrentOperation,
				"report-error":      reportErrors,
>>>>>>> 3.6
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
<<<<<<< HEAD
			err := a.ops.RefreshApplicationStatus(ctx, name, a.appUUID, app, appLife,
				a.statusService, a.clock, a.logger)
			if err != nil {
				return errors.Annotatef(err, "refreshing application status for %q", name)
=======
			if err = a.ops.RefreshApplicationStatus(a.name, state.app, appLife, a.facade, a.logger); err != nil {
				return errors.Annotatef(err, "refreshing application status for %q", a.name)
>>>>>>> 3.6
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
			err := a.ops.AppDying(a.name, state.app, state.appLife, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		}
		state.ready = false

	case life.Dead:
		if !a.statusOnly {
			err := a.ops.AppDying(a.name, state.app, state.appLife, a.facade, a.unitFacade, a.logger)
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
		return errors.NotImplementedf("unknown life %q", state.appLife)
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
	state.appLife = appLife

	switch appLife {
	case life.Alive:
		err := a.ops.EnsureStorage(a.name, state.app, a.password, a.facade,
			a.clock, a.logger)
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

// scopedContext returns a context that is in the scope of the watcher lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (a *appWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return a.catacomb.Context(ctx), cancel
}
