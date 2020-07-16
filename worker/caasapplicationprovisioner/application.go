// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"strings"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/utils"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type appWorker struct {
	catacomb catacomb.Catacomb
	facade   CAASProvisionerFacade
	broker   caas.Broker
	clock    clock.Clock
	logger   Logger

	name     string
	modelTag names.ModelTag
	changes  chan struct{}
}

type appWorkerConfig struct {
	Name     string
	Facade   CAASProvisionerFacade
	Broker   caas.Broker
	ModelTag names.ModelTag
	Clock    clock.Clock
	Logger   Logger
}

func newAppWorker(config appWorkerConfig) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		changes := make(chan struct{}, 1)
		changes <- struct{}{}
		a := &appWorker{
			name:     config.Name,
			facade:   config.Facade,
			broker:   config.Broker,
			modelTag: config.ModelTag,
			clock:    config.Clock,
			logger:   config.Logger,
			changes:  changes,
		}
		err := catacomb.Invoke(catacomb.Plan{
			Site: &a.catacomb,
			Work: a.loop,
		})
		return a, err
	}
}

func (a *appWorker) Notify() {
	a.changes <- struct{}{}
}

func (a *appWorker) Kill() {
	a.catacomb.Kill(nil)
}

func (a *appWorker) Wait() error {
	return a.catacomb.Wait()
}

func (a *appWorker) loop() error {
	var err error
	var appLife life.Value = life.Dead

	charmURL, err := a.facade.ApplicationCharmURL(a.name)
	if errors.IsNotFound(err) {
		// Application might be dead.
	} else if err != nil {
		return errors.Annotatef(err, "failed to get charm urls for application")
	} else {
		charmInfo, err := a.facade.CharmInfo(charmURL.String())
		if err != nil {
			return errors.Annotatef(err, "failed to get application charm deployment metadata for %q", a.name)
		}
		if charmInfo == nil ||
			charmInfo.Meta == nil ||
			charmInfo.Meta.Deployment == nil ||
			charmInfo.Meta.Deployment.DeploymentMode != charm.ModeEmbedded {
			a.logger.Debugf("skipping non-embedded application %q", a.name)
			a.catacomb.Kill(nil)
			return nil
		}
	}

	for {
		select {
		case <-a.catacomb.Dead():
			return a.catacomb.Err()
		case <-a.changes:
			appLife, err = a.facade.Life(a.name)
			if errors.IsNotFound(err) {
				appLife = life.Dead
			} else if err != nil {
				return errors.Trace(err)
			}
			switch appLife {
			case life.Alive:
				err = a.alive()
				if err != nil {
					return errors.Trace(err)
				}
			case life.Dying:
				err = a.dying()
				if err != nil {
					return errors.Trace(err)
				}
			case life.Dead:
				return a.dead()
			}
		}
	}
}

func (a *appWorker) alive() error {
	charmURL, err := a.facade.ApplicationCharmURL(a.name)
	if err != nil {
		return errors.Annotatef(err, "failed to get charm urls for application")
	}

	charmInfo, err := a.facade.CharmInfo(charmURL.String())
	if err != nil {
		return errors.Annotatef(err, "failed to get application charm deployment metadata for %q", a.name)
	}

	config := &caas.ApplicationConfig{
		ApplicationName: a.name,
		ModelUUID:       a.modelTag.Id(),
	}

	appState, err := a.broker.ApplicationExists(config)
	if err != nil {
		return errors.Annotatef(err, "failed to find application for %q", a.name)
	}
	if appState.Exists && appState.Terminating {
		if err := a.waitForTerminated(config); err != nil {
			return errors.Annotatef(err, "%q was terminating and there was an error waiting for it to stop", a.name)
		}
	}

	password, err := utils.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	err = a.facade.SetPassword(a.name, password)
	if err != nil {
		return errors.Annotate(err, "failed to set application api passwords")
	}

	provisionInfo, err := a.facade.ProvisioningInfo(a.name)
	if err != nil {
		return errors.Annotate(err, "failed to get provisioning info")
	}

	config.Charm = charmInfo.Charm()
	config.IntroductionSecret = password
	config.AgentVersion = provisionInfo.Version
	config.AgentImagePath = provisionInfo.ImagePath
	config.ControllerAddresses = strings.Join(provisionInfo.APIAddresses, ",")
	config.ControllerCertBundle = provisionInfo.CACert
	config.ResourceTags = provisionInfo.Tags

	err = a.broker.EnsureApplication(config)
	if err != nil {
		return errors.Annotate(err, "ensuring application")
	}

	reason := "deployed"
	if appState.Exists {
		reason = "updated"
	}
	err = a.facade.SetOperatorStatus(a.name, status.Active, reason, nil)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (a *appWorker) dying() error {
	config := &caas.ApplicationConfig{
		ApplicationName: a.name,
		ModelUUID:       a.modelTag.Id(),
	}
	err := a.broker.DeleteApplication(config)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *appWorker) dead() error {
	err := a.dying()
	if err != nil {
		return errors.Trace(err)
	}
	config := &caas.ApplicationConfig{
		ApplicationName: a.name,
		ModelUUID:       a.modelTag.Id(),
	}
	err = a.waitForTerminated(config)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *appWorker) waitForTerminated(config *caas.ApplicationConfig) error {
	tryAgain := errors.New("try again")
	existsFunc := func() error {
		appState, err := a.broker.ApplicationExists(config)
		if err != nil {
			return errors.Trace(err)
		}
		if !appState.Exists {
			return nil
		}
		if appState.Exists && !appState.Terminating {
			return errors.Errorf("application %q should be terminating but is now running", config.ApplicationName)
		}
		return tryAgain
	}
	retryCallArgs := retry.CallArgs{
		Attempts:    60,
		Delay:       3 * time.Second,
		MaxDuration: 3 * time.Minute,
		Clock:       a.clock,
		Func:        existsFunc,
		IsFatalError: func(err error) bool {
			return err != tryAgain
		},
	}
	return errors.Trace(retry.Call(retryCallArgs))
}
