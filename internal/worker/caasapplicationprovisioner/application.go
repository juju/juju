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
	// TODO(sidecar): support more than statefulset
	app := a.broker.Application(a.name, caas.DeploymentStateful)

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
			err = a.ops.AppDying(a.name, app, a.life, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Annotatef(err, "deleting application %q", a.name)
			}
			err = a.ops.AppDead(a.name, app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
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
	}

	var appChanges watcher.NotifyChannel
	var appProvisionChanges watcher.NotifyChannel
	var replicaChanges watcher.NotifyChannel
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
		return errors.Annotatef(err, "creating application %q storage constraints watcher", a.name)
	}
	if err := a.catacomb.Add(storageConstraintsWatcher); err != nil {
		return errors.Annotatef(err, "failed to watch for application %q storage constraints changes", a.name)
	}

	done := false

	var (
		initial                = true
		scaleChan              <-chan time.Time
		scaleTries             int
		trustChan              <-chan time.Time
		trustTries             int
		reconcileDeadChan      <-chan time.Time
		stateAppChangedChan    <-chan time.Time
		storageConstraintsChan <-chan time.Time
	)
	const (
		maxRetries = 20
		retryDelay = 3 * time.Second
	)

	handleChange := func() error {
		appLife, err = a.facade.Life(a.name)
		if errors.Is(err, errors.NotFound) {
			appLife = life.Dead
		} else if err != nil {
			return errors.Trace(err)
		}
		a.life = appLife

		if initial {
			initial = false
			ps, err := a.facade.ProvisioningState(a.name)
			if err != nil {
				return errors.Trace(err)
			}
			if ps != nil && ps.Scaling {
				if a.statusOnly {
					// Clear provisioning state for status only app.
					err = a.facade.SetProvisioningState(a.name, params.CAASApplicationProvisioningState{})
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
				appProvisionWatcher, err := a.facade.WatchProvisioningInfo(a.name)
				if err != nil {
					return errors.Annotatef(err, "failed to watch facade for changes to application provisioning %q", a.name)
				}
				if err := a.catacomb.Add(appProvisionWatcher); err != nil {
					return errors.Trace(err)
				}
				appProvisionChanges = appProvisionWatcher.Changes()
			}
			if !a.statusOnly {
				err = a.ops.AppAlive(a.name, app, a.password, &a.lastApplied, a.facade, a.clock, a.logger)
				if errors.Is(err, errors.NotProvisioned) {
					// State not ready for this application to be provisioned yet.
					// Usually because the charm has not yet been downloaded.
					break
				} else if err != nil {
					return errors.Trace(err)
				}
			}
			if appChanges == nil {
				appWatcher, err := app.Watch()
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
		case life.Dying:
			if !a.statusOnly {
				err = a.ops.AppDying(a.name, app, a.life, a.facade, a.unitFacade, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
			}
		case life.Dead:
			if !a.statusOnly {
				err = a.ops.AppDying(a.name, app, a.life, a.facade, a.unitFacade, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
				err = a.ops.AppDead(a.name, app, a.broker, a.facade, a.unitFacade, a.clock, a.logger)
				if err != nil {
					return errors.Trace(err)
				}
			}
			done = true
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
			err := a.ops.EnsureScale(a.name, app, a.life, a.facade, a.unitFacade, a.logger)
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
			err := a.ops.EnsureTrust(a.name, app, a.unitFacade, a.logger)
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
			err := a.ops.ReconcileDeadUnitScale(a.name, app, a.facade, a.logger)
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
			lastReportedStatus, err = a.ops.UpdateState(a.name, app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		case <-replicaChanges:
			// Respond to changes in replicas of the application.
			lastReportedStatus, err = a.ops.UpdateState(a.name, app, lastReportedStatus, a.broker, a.facade, a.unitFacade, a.logger)
			if err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-storageConstraintsWatcher.Changes():
			if !ok {
				return fmt.Errorf("application %q storage constraints watcher closed channel", a.name)
			}
			if storageConstraintsChan == nil {
				storageConstraintsChan = a.clock.After(0)
			}
			shouldRefresh = false
		case <-a.clock.After(10 * time.Second):
			// Force refresh of application status.
		}
		if done {
			return nil
		}
		if shouldRefresh {
			if err = a.ops.RefreshApplicationStatus(a.name, app, appLife, a.facade, a.logger); err != nil {
				return errors.Annotatef(err, "refreshing application status for %q", a.name)
			}
		}
	}
}
