// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"strings"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/systems"
	"github.com/juju/utils/v2"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

type appNotifyWorker interface {
	worker.Worker
	Notify()
}

type appWorker struct {
	catacomb catacomb.Catacomb
	facade   CAASProvisionerFacade
	broker   CAASBroker
	clock    clock.Clock
	logger   Logger

	name     string
	modelTag names.ModelTag
	changes  chan struct{}
}

type AppWorkerConfig struct {
	Name     string
	Facade   CAASProvisionerFacade
	Broker   CAASBroker
	ModelTag names.ModelTag
	Clock    clock.Clock
	Logger   Logger
}

type NewAppWorkerFunc func(AppWorkerConfig) func() (worker.Worker, error)

func NewAppWorker(config AppWorkerConfig) func() (worker.Worker, error) {
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
	if err != nil {
		return errors.Annotatef(err, "failed to get charm url for application")
	}
	charmInfo, err := a.facade.CharmInfo(charmURL.String())
	if err != nil {
		return errors.Annotatef(err, "failed to get application charm deployment metadata for %q", a.name)
	}
	if charmInfo == nil ||
		charmInfo.Meta == nil ||
		charmInfo.Meta.Format() < charm.FormatV2 {
		return errors.Errorf("charm version 2 or greater required")
	}

	// TODO(new-charms): support more than statefulset
	app := a.broker.Application(a.name, caas.DeploymentStateful)

	var appChanges watcher.NotifyChannel
	var replicaChanges watcher.NotifyChannel

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
				err = a.alive(app)
				if err != nil {
					return errors.Trace(err)
				}
				if appChanges == nil {
					appWatcher, err := app.Watch()
					if err != nil {
						return errors.Annotatef(err, "failed to watch for changes to application %q", a.name)
					}
					err = a.catacomb.Add(appWatcher)
					if err != nil {
						appWatcher.Kill()
						return errors.Trace(err)
					}
					appChanges = appWatcher.Changes()
				}
				if replicaChanges == nil {
					replicaWatcher, err := app.WatchReplicas()
					if err != nil {
						return errors.Annotatef(err, "failed to watch for changes to replicas %q", a.name)
					}
					err = a.catacomb.Add(replicaWatcher)
					if err != nil {
						replicaWatcher.Kill()
						return errors.Trace(err)
					}
					replicaChanges = replicaWatcher.Changes()
				}
			case life.Dying:
				err = a.dying(app)
				if err != nil {
					return errors.Trace(err)
				}
			case life.Dead:
				return a.dead(app)
			}
		case <-appChanges:
			err = a.updateState(app, false)
			if err != nil {
				return errors.Trace(err)
			}
		case <-replicaChanges:
			err = a.updateState(app, false)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (a *appWorker) updateState(app caas.Application, force bool) error {
	// Fetching the units here is to ensure happens-before consistency
	// on the deletion of units.
	observedUnits, err := a.facade.Units(a.name)
	if err != nil {
		return errors.Trace(err)
	}
	st, err := app.State()
	if errors.IsNotFound(err) {
		// Do nothing
	} else if err != nil {
		return errors.Trace(err)
	}
	err = a.facade.GarbageCollect(a.name, observedUnits, st.DesiredReplicas, st.Replicas, force)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *appWorker) alive(app caas.Application) error {
	charmURL, err := a.facade.ApplicationCharmURL(a.name)
	if err != nil {
		return errors.Annotatef(err, "failed to get charm urls for application")
	}

	charmInfo, err := a.facade.CharmInfo(charmURL.String())
	if err != nil {
		return errors.Annotatef(err, "failed to get application charm deployment metadata for %q", a.name)
	}

	appState, err := app.Exists()
	if err != nil {
		return errors.Annotatef(err, "failed get application state for %q", a.name)
	}

	if appState.Exists && appState.Terminating {
		if err := a.waitForTerminated(app); err != nil {
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

	images, err := a.facade.ApplicationOCIResources(a.name)
	if err != nil {
		return errors.Annotate(err, "failed to get oci image resources")
	}

	baseSystem, err := systems.ParseSystemFromSeries(provisionInfo.Series)
	if err != nil {
		return errors.Annotate(err, "failed to parse series as a system")
	}

	ch := charmInfo.Charm()
	charmBaseImage := resources.DockerImageDetails{}
	if baseSystem.Resource != "" {
		image, ok := images[baseSystem.Resource]
		if !ok {
			return errors.NotFoundf("referenced charm base image resource %s", baseSystem.Resource)
		}
		charmBaseImage = image
	} else {
		charmBaseImage.RegistryPath, err = podcfg.ImageForSystem(provisionInfo.ImageRepo, baseSystem)
		if err != nil {
			return errors.Annotate(err, "failed to get image for system")
		}
	}

	containers := make(map[string]caas.ContainerConfig)
	for k, v := range ch.Meta().Containers {
		container := caas.ContainerConfig{
			Name: k,
		}
		if len(v.Systems) != 1 {
			return errors.NotValidf("containers currently only support declaring one system")
		}
		system := v.Systems[0]
		if system.Resource != "" {
			image, ok := images[system.Resource]
			if !ok {
				return errors.NotFoundf("referenced charm base image resource %s", system.Resource)
			}
			container.Image = image
		} else {
			container.Image.RegistryPath, err = podcfg.ImageForSystem(provisionInfo.ImageRepo, system)
			if err != nil {
				return errors.Annotate(err, "failed to get image for system")
			}
		}
		for _, m := range v.Mounts {
			container.Mounts = append(container.Mounts, caas.MountConfig{
				StorageName: m.Storage,
				Path:        m.Location,
			})
		}
		containers[k] = container
	}

	config := caas.ApplicationConfig{
		IntroductionSecret:   password,
		AgentVersion:         provisionInfo.Version,
		AgentImagePath:       provisionInfo.ImagePath,
		ControllerAddresses:  strings.Join(provisionInfo.APIAddresses, ","),
		ControllerCertBundle: provisionInfo.CACert,
		ResourceTags:         provisionInfo.Tags,
		Constraints:          provisionInfo.Constraints,
		Filesystems:          provisionInfo.Filesystems,
		Devices:              provisionInfo.Devices,
		CharmBaseImage:       charmBaseImage,
		Containers:           containers,
	}
	err = app.Ensure(config)
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

func (a *appWorker) dying(app caas.Application) error {
	err := app.Delete()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *appWorker) dead(app caas.Application) error {
	err := a.dying(app)
	if err != nil {
		return errors.Trace(err)
	}
	err = a.waitForTerminated(app)
	if err != nil {
		return errors.Trace(err)
	}
	err = a.updateState(app, true)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *appWorker) waitForTerminated(app caas.Application) error {
	tryAgain := errors.New("try again")
	existsFunc := func() error {
		appState, err := app.Exists()
		if err != nil {
			return errors.Trace(err)
		}
		if !appState.Exists {
			return nil
		}
		if appState.Exists && !appState.Terminating {
			return errors.Errorf("application %q should be terminating but is now running", a.name)
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
