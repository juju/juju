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
	"github.com/juju/juju/core/k8s"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/password"
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
	logger     logger.Logger
	unitFacade CAASUnitProvisionerFacade
	ops        ApplicationOps

	name        string
	modelTag    names.ModelTag
	changes     chan struct{}
	password    string
	lastApplied caas.ApplicationConfig
	life        life.Value
	statusOnly  bool
}

type AppWorkerConfig struct {
	Name       string
	Facade     CAASProvisionerFacade
	Broker     CAASBroker
	ModelTag   names.ModelTag
	Clock      clock.Clock
	Logger     logger.Logger
	UnitFacade CAASUnitProvisionerFacade
	Ops        ApplicationOps
	StatusOnly bool
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
			name:       config.Name,
			facade:     config.Facade,
			broker:     config.Broker,
			modelTag:   config.ModelTag,
			clock:      config.Clock,
			logger:     config.Logger,
			changes:    changes,
			unitFacade: config.UnitFacade,
			ops:        ops,
			statusOnly: config.StatusOnly,
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
	ctx, cancel := a.scopedContext()
	defer cancel()

	// TODO(sidecar): support more than statefulset
	app := a.broker.Application(a.name, k8s.WorkloadTypeStatefulSet)

	// If the application no longer exists, return immediately. If it's in
	// Dead state, ensure it's deleted and terminated.
	appLife, err := a.facade.Life(ctx, a.name)
	if errors.Is(err, errors.NotFound) {
		a.logger.Debugf(ctx, "application %q no longer exists", a.name)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "fetching life status for application %q", a.name)
	}
	a.life = appLife
	if appLife == life.Dead {
		if !a.statusOnly {
			err = a.ops.AppDying(ctx, a.name, app, a.life, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", a.name)
			}
			err = a.ops.AppDead(ctx, a.name, app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", a.name)
			}
		}
		return nil
	}

	// Ensure the charm is to a v2 charm.
	isOk, err := a.ops.CheckCharmFormat(ctx, a.name, a.facade, a.logger)
	if !isOk || err != nil {
		return errors.Trace(err)
	}

	if !a.statusOnly {
		// Update the password once per worker start to avoid it changing too frequently.
		a.password, err = password.RandomPassword()
		if err != nil {
			return errors.Trace(err)
		}
		err = a.facade.SetPassword(ctx, a.name, a.password)
		if err != nil {
			return errors.Annotate(err, "failed to set application api passwords")
		}
	}

	var appChanges watcher.NotifyChannel
	var appProvisionChanges watcher.NotifyChannel
	var replicaChanges watcher.NotifyChannel
	var lastReportedStatus map[string]status.StatusInfo

	appScaleWatcher, err := a.unitFacade.WatchApplicationScale(ctx, a.name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q scale watcher", a.name)
	}
	if err := a.catacomb.Add(appScaleWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q scale changes", a.name)
	}

	appTrustWatcher, err := a.unitFacade.WatchApplicationTrustHash(ctx, a.name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q trust watcher", a.name)
	}
	if err := a.catacomb.Add(appTrustWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q trust changes", a.name)
	}

	var appUnitsWatcher watcher.StringsWatcher
	appUnitsWatcher, err = a.facade.WatchUnits(ctx, a.name)
	if err != nil {
		return errors.Annotatef(err, "creating application %q units watcher", a.name)
	}
	if err := a.catacomb.Add(appUnitsWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q units changes", a.name)
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
		appLife, err = a.facade.Life(ctx, a.name)
		if errors.Is(err, errors.NotFound) {
			appLife = life.Dead
		} else if err != nil {
			return errors.Trace(err)
		}
		a.life = appLife

		if initial {
			initial = false
			ps, err := a.facade.ProvisioningState(ctx, a.name)
			if err != nil {
				return errors.Trace(err)
			}
			if ps != nil && ps.Scaling {
				if a.statusOnly {
					// Clear provisioning state for status only app.
					err = a.facade.SetProvisioningState(ctx, a.name, params.CAASApplicationProvisioningState{})
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
				appProvisionWatcher, err := a.facade.WatchProvisioningInfo(ctx, a.name)
				if err != nil {
					return errors.Annotatef(err, "failed to watch facade for changes to application provisioning %q", a.name)
				}
				if err := a.catacomb.Add(appProvisionWatcher); err != nil {
					return errors.Trace(err)
				}
				appProvisionChanges = appProvisionWatcher.Changes()
			}
			if !a.statusOnly {
				err = a.ops.AppAlive(ctx, a.name, app, a.password, &a.lastApplied, a.facade, a.clock, a.logger)
				if errors.Is(err, errors.NotProvisioned) {
					a.logger.Debugf(ctx, "application %q is not provisioned", a.name)
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
					return errors.Annotatef(err, "failed to watch for changes to application %q", a.name)
				}
				if err := a.catacomb.Add(appWatcher); err != nil {
					return errors.Trace(err)
				}
				appChanges = appWatcher.Changes()
			}
			if replicaChanges == nil {
				replicaWatcher, err := app.WatchReplicas()
				if err != nil {
					return errors.Annotatef(err, "failed to watch for changes to replicas %q", a.name)
				}
				if err := a.catacomb.Add(replicaWatcher); err != nil {
					return errors.Trace(err)
				}
				replicaChanges = replicaWatcher.Changes()
			}
			a.logger.Debugf(ctx, "application %q is ready", a.name)
			ready = true
		case life.Dying:
			if !a.statusOnly {
				err = a.ops.AppDying(ctx, a.name, app, a.life, a.facade, a.unitFacade, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
			}
			ready = false
		case life.Dead:
			if !a.statusOnly {
				err = a.ops.AppDying(ctx, a.name, app, a.life, a.facade, a.unitFacade, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
				err = a.ops.AppDead(ctx, a.name, app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
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
				return fmt.Errorf("application %q scale watcher closed channel", a.name)
			}
			if scaleChan == nil {
				scaleTries = 0
				scaleChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-scaleChan:
			if a.statusOnly {
				scaleChan = nil
				break
			}
			if !ready {
				scaleChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
			err := a.ops.EnsureScale(ctx, a.name, app, a.life, a.facade, a.unitFacade, a.logger)
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
		case _, ok := <-appTrustWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q trust watcher closed channel", a.name)
			}
			if trustChan == nil {
				trustTries = 0
				trustChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-trustChan:
			if a.statusOnly {
				trustChan = nil
				break
			}
			if !ready {
				trustChan = a.clock.After(retryDelay)
				shouldRefresh = false
				break
			}
			err := a.ops.EnsureTrust(ctx, a.name, app, a.unitFacade, a.logger)
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
				return fmt.Errorf("application %q units watcher closed channel", a.name)
			}
			if reconcileDeadChan == nil {
				reconcileDeadChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-reconcileDeadChan:
			if a.statusOnly {
				reconcileDeadChan = nil
				break
			}
			err := a.ops.ReconcileDeadUnitScale(ctx, a.name, app, a.facade, a.logger)
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
			lastReportedStatus, err = a.ops.UpdateState(ctx, a.name, app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		case <-replicaChanges:
			// Respond to changes in replicas of the application.
			lastReportedStatus, err = a.ops.UpdateState(ctx, a.name, app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
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
			if err = a.ops.RefreshApplicationStatus(ctx, a.name, app, appLife, a.facade, a.logger); err != nil {
				return errors.Annotatef(err, "refreshing application status for %q", a.name)
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
