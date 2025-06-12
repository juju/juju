// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/password"
)

type appNotifyWorker interface {
	worker.Worker
	Notify()
}

type appWorker struct {
	catacomb           catacomb.Catacomb
	applicationService ApplicationService
	statusService      StatusService
	facade             CAASProvisionerFacade
	broker             CAASBroker
	clock              clock.Clock
	logger             logger.Logger
	unitFacade         CAASUnitProvisionerFacade
	ops                ApplicationOps

	appID       coreapplication.ID
	modelTag    names.ModelTag
	changes     chan struct{}
	password    string
	lastApplied caas.ApplicationConfig
	life        life.Value
}

type AppWorkerConfig struct {
	AppID coreapplication.ID

	ApplicationService ApplicationService
	StatusService      StatusService

	Ops    ApplicationOps
	Broker CAASBroker
	Clock  clock.Clock
	Logger logger.Logger

	// TODO: remove these
	Facade     CAASProvisionerFacade
	UnitFacade CAASUnitProvisionerFacade
	ModelTag   names.ModelTag
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
			applicationService: config.ApplicationService,
			statusService:      config.StatusService,
			appID:              config.AppID,
			facade:             config.Facade,
			broker:             config.Broker,
			modelTag:           config.ModelTag,
			clock:              config.Clock,
			logger:             config.Logger,
			changes:            changes,
			unitFacade:         config.UnitFacade,
			ops:                ops,
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
	name, err := a.applicationService.GetApplicationName(ctx, a.appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		a.logger.Debugf(ctx, "application %q no longer exists", a.appID)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "fetching info for application %q", a.appID)
	}

	// TODO: figure out how to do this.
	statusOnly := name == "controller"

	// TODO(sidecar): support more than statefulset
	app := a.broker.Application(name, caas.DeploymentStateful)

	// If the application no longer exists, return immediately. If it's in
	// Dead state, ensure it's deleted and terminated.
	appLife, err := a.applicationService.GetApplicationLife(ctx, a.appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		a.logger.Debugf(ctx, "application %q no longer exists", name)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "fetching life status for application %q", name)
	}
	a.life = appLife
	if appLife == life.Dead {
		if !statusOnly {
			err = a.ops.AppDying(ctx, name, a.appID, app, a.life, a.facade, a.unitFacade, a.applicationService, a.statusService, a.logger)
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", name)
			}
			err = a.ops.AppDead(ctx, name, app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", name)
			}
		}
		return nil
	}

	// Ensure the charm is to a v2 charm.
	isOk, err := a.ops.CheckCharmFormat(ctx, name, a.facade, a.logger)
	if !isOk || err != nil {
		return errors.Trace(err)
	}

	if !statusOnly {
		// Update the password once per worker start to avoid it changing too frequently.
		a.password, err = password.RandomPassword()
		if err != nil {
			return errors.Trace(err)
		}
		err = a.facade.SetPassword(ctx, name, a.password)
		if err != nil {
			return errors.Annotate(err, "failed to set application api passwords")
		}
	}

	var appChanges watcher.NotifyChannel
	var appProvisionChanges watcher.NotifyChannel
	var replicaChanges watcher.NotifyChannel
	var lastReportedStatus map[string]status.StatusInfo

	appScaleWatcher, err := a.unitFacade.WatchApplicationScale(ctx, name)
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

	var (
		done                = false // done is true when the app is dead and cleaned up.
		ready               = false // ready is true when the k8s resources are created.
		initial             = true
		scaleChan           <-chan time.Time
		scaleTries          int
		trustChan           <-chan time.Time
		trustTries          int
		reconcileDeadChan   <-chan time.Time
		stateAppChangedChan <-chan time.Time
	)
	const (
		maxRetries = 20
		retryDelay = 3 * time.Second
	)

	handleChange := func() error {
		appLife, err := a.applicationService.GetApplicationLife(ctx, a.appID)
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
				err = a.ops.AppAlive(ctx, name, app, a.password, &a.lastApplied, a.facade, a.statusService, a.clock, a.logger)
				if errors.Is(err, errors.NotProvisioned) {
					a.logger.Debugf(ctx, "application %q is not provisioned", name)
					// State not ready for this application to be provisioned yet.
					// Usually because the charm has not yet been downloaded.
					return tryAgain
				} else if err != nil {
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
				err = a.ops.AppDying(ctx, name, a.appID, app, a.life, a.facade, a.unitFacade, a.applicationService, a.statusService, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
			}
			ready = false
		case life.Dead:
			if !statusOnly {
				err = a.ops.AppDying(ctx, name, a.appID, app, a.life, a.facade, a.unitFacade, a.applicationService, a.statusService, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
				err = a.ops.AppDead(ctx, name, app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
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

	for {
		shouldRefresh := true
		select {
		case _, ok := <-appScaleWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q scale watcher closed channel", name)
			}
			if scaleChan == nil {
				scaleTries = 0
				scaleChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-scaleChan:
			if statusOnly {
				scaleChan = nil
				break
			}
			if !ready {
				scaleChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
			err := a.ops.EnsureScale(ctx, name, a.appID, app, a.life, a.facade, a.unitFacade, a.applicationService, a.statusService, a.logger)
			if errors.Is(err, errors.NotFound) {
				if scaleTries >= maxRetries {
					return errors.Annotatef(err, "more than %d retries ensuring scale", maxRetries)
				}
				scaleTries++
				scaleChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if errors.Is(err, tryAgain) {
				scaleChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return errors.Trace(err)
			} else {
				scaleChan = nil
			}
		case _, ok := <-appSettingsWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q trust watcher closed channel", name)
			}
			if trustChan == nil {
				trustTries = 0
				trustChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-trustChan:
			if statusOnly {
				trustChan = nil
				break
			}
			if !ready {
				trustChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
			err := a.ops.EnsureTrust(ctx, name, app, a.applicationService, a.logger)
			if errors.Is(err, errors.NotFound) {
				if trustTries >= maxRetries {
					return errors.Annotatef(err, "more than %d retries ensuring trust", maxRetries)
				}
				trustTries++
				trustChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return errors.Trace(err)
			} else {
				trustChan = nil
			}
		case _, ok := <-appUnitsWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q units watcher closed channel", name)
			}
			if reconcileDeadChan == nil {
				reconcileDeadChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-reconcileDeadChan:
			if statusOnly {
				reconcileDeadChan = nil
				break
			}
			err := a.ops.ReconcileDeadUnitScale(ctx, name, a.appID, app, a.facade, a.applicationService, a.statusService, a.logger)
			if errors.Is(err, errors.NotFound) {
				reconcileDeadChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if errors.Is(err, tryAgain) {
				reconcileDeadChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return fmt.Errorf("reconciling dead unit scale: %w", err)
			} else {
				reconcileDeadChan = nil
			}
		case <-a.catacomb.Dying():
			return a.catacomb.ErrDying()
		case <-appProvisionChanges:
			if stateAppChangedChan == nil {
				stateAppChangedChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-a.changes:
			if stateAppChangedChan == nil {
				stateAppChangedChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-stateAppChangedChan:
			// Respond to life changes (Notify called by parent worker).
			err = handleChange()
			if errors.Is(err, tryAgain) {
				stateAppChangedChan = a.clock.After(retryDelay)
				shouldRefresh = false
			} else if err != nil {
				return errors.Trace(err)
			} else {
				stateAppChangedChan = nil
			}
		case <-appChanges:
			// Respond to changes in provider application.
			lastReportedStatus, err = a.ops.UpdateState(ctx, name, app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		case <-replicaChanges:
			// Respond to changes in replicas of the application.
			lastReportedStatus, err = a.ops.UpdateState(ctx, name, app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		case <-a.clock.After(10 * time.Second):
			// Force refresh of application status.
		}
		if done {
			return nil
		}
		if shouldRefresh {
			if err = a.ops.RefreshApplicationStatus(ctx, name, a.appID, app, appLife, a.facade, a.statusService, a.clock, a.logger); err != nil {
				return errors.Annotatef(err, "refreshing application status for %q", name)
			}
		}
	}
}

// scopedContext returns a context that is in the scope of the watcher lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (a *appWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return a.catacomb.Context(ctx), cancel
}
