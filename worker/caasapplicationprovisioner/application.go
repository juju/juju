// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"reflect"
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

	"github.com/juju/juju/apiserver/params"
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

	name        string
	modelTag    names.ModelTag
	changes     chan struct{}
	password    string
	lastApplied caas.ApplicationConfig
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
	charmURL, err := a.facade.ApplicationCharmURL(a.name)
	if errors.IsNotFound(err) {
		a.logger.Debugf("application %q removed", a.name)
		return nil
	} else if err != nil {
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

	// Update the password once per worker start to avoid it changing too frequently.
	a.password, err = utils.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	err = a.facade.SetPassword(a.name, a.password)
	if err != nil {
		return errors.Annotate(err, "failed to set application api passwords")
	}

	// TODO(embedded): support more than statefulset
	app := a.broker.Application(a.name, caas.DeploymentStateful)

	var appLife life.Value
	var appChanges watcher.NotifyChannel
	var replicaChanges watcher.NotifyChannel
	var appStateChanges watcher.NotifyChannel
	var lastReportedStatus map[string]status.StatusInfo

	done := false

	handleChange := func() error {
		appLife, err = a.facade.Life(a.name)
		if errors.IsNotFound(err) {
			appLife = life.Dead
		} else if err != nil {
			return errors.Trace(err)
		}
		switch appLife {
		case life.Alive:
			if appStateChanges == nil {
				appStateWatcher, err := a.facade.WatchApplication(a.name)
				if err != nil {
					return errors.Annotatef(err, "failed to watch for changes to application %q", a.name)
				}
				if err := a.catacomb.Add(appStateWatcher); err != nil {
					return errors.Trace(err)
				}
				appStateChanges = appStateWatcher.Changes()
			}
			err = a.alive(app)
			if err != nil {
				return errors.Trace(err)
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
			err = a.dying(app)
			if err != nil {
				return errors.Trace(err)
			}
		case life.Dead:
			err = a.dead(app)
			if err != nil {
				return errors.Trace(err)
			}
			done = true
			return nil
		}
		return nil
	}

	for {
		select {
		case <-a.catacomb.Dying():
			return a.catacomb.ErrDying()
		case <-appStateChanges:
			// Respond to state changes
			err = handleChange()
			if err != nil {
				return errors.Trace(err)
			}
		case <-a.changes:
			// Respond to life changes
			err = handleChange()
			if err != nil {
				return errors.Trace(err)
			}
		case <-appChanges:
			// Respond to changes in provider application
			lastReportedStatus, err = a.updateState(app, false, lastReportedStatus)
			if err != nil {
				return errors.Trace(err)
			}
		case <-replicaChanges:
			// Respond to changes in replicas of the application
			lastReportedStatus, err = a.updateState(app, false, lastReportedStatus)
			if err != nil {
				return errors.Trace(err)
			}
		}
		if done {
			return nil
		}
	}
}

func (a *appWorker) updateState(app caas.Application, force bool, lastReportedStatus map[string]status.StatusInfo) (map[string]status.StatusInfo, error) {
	// Fetching the units here is to ensure happens-before consistency
	// on the deletion of units.
	observedUnits, err := a.facade.Units(a.name)
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	st, err := app.State()
	if errors.IsNotFound(err) {
		// Do nothing
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	err = a.facade.GarbageCollect(a.name, observedUnits, st.DesiredReplicas, st.Replicas, force)
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	if force {
		return nil, nil
	}
	// TODO: consolidate GarbageCollect and UpdateApplicationUnits into a single call.
	units, err := app.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}

	reportedStatus := make(map[string]status.StatusInfo)
	args := params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag(a.name).String(),
		Status:         params.EntityStatus{},
	}
	for _, u := range units {
		// For pods managed by the substrate, any marked as dying
		// are treated as non-existing.
		if u.Dying {
			continue
		}
		unitStatus := u.Status
		lastStatus, ok := lastReportedStatus[u.Id]
		reportedStatus[u.Id] = unitStatus
		// TODO: Determine a better way to propagate status
		// without constantly overriding the juju state value.
		if ok {
			// If we've seen the same status value previously,
			// report as unknown as this value is ignored.
			if reflect.DeepEqual(lastStatus, unitStatus) {
				unitStatus = status.StatusInfo{
					Status: status.Unknown,
				}
			}
		}
		unitParams := params.ApplicationUnitParams{
			ProviderId: u.Id,
			Address:    u.Address,
			Ports:      u.Ports,
			Stateful:   u.Stateful,
			Status:     unitStatus.Status.String(),
			Info:       unitStatus.Message,
			Data:       unitStatus.Data,
		}
		// Fill in any filesystem info for volumes attached to the unit.
		// A unit will not become active until all required volumes are
		// provisioned, so it makes sense to send this information along
		// with the units to which they are attached.
		for _, info := range u.FilesystemInfo {
			unitParams.FilesystemInfo = append(unitParams.FilesystemInfo, params.KubernetesFilesystemInfo{
				StorageName:  info.StorageName,
				FilesystemId: info.FilesystemId,
				Size:         info.Size,
				MountPoint:   info.MountPoint,
				ReadOnly:     info.ReadOnly,
				Status:       info.Status.Status.String(),
				Info:         info.Status.Message,
				Data:         info.Status.Data,
				Volume: params.KubernetesVolumeInfo{
					VolumeId:   info.Volume.VolumeId,
					Size:       info.Volume.Size,
					Persistent: info.Volume.Persistent,
					Status:     info.Volume.Status.Status.String(),
					Info:       info.Volume.Status.Message,
					Data:       info.Volume.Status.Data,
				},
			})
		}
		args.Units = append(args.Units, unitParams)
	}

	appUnitInfo, err := a.facade.UpdateUnits(args)
	if err != nil {
		// We can ignore not found errors as the worker will get stopped anyway.
		// We can also ignore Forbidden errors raised from SetScale because disordered events could happen often.
		if !errors.IsForbidden(err) && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		a.logger.Warningf("update units %v", err)
	}

	if appUnitInfo != nil {
		for _, unitInfo := range appUnitInfo.Units {
			unit, err := names.ParseUnitTag(unitInfo.UnitTag)
			if err != nil {
				return nil, errors.Trace(err)
			}
			err = a.broker.AnnotateUnit(a.name, caas.ModeEmbedded, unitInfo.ProviderId, unit)
			if errors.IsNotFound(err) {
				continue
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}
	}
	return reportedStatus, nil
}

func (a *appWorker) alive(app caas.Application) error {
	a.logger.Debugf("ensuring application %q exists", a.name)

	provisionInfo, err := a.facade.ProvisioningInfo(a.name)
	if err != nil {
		return errors.Annotate(err, "failed to get provisioning info")
	}
	if provisionInfo.CharmURL == nil {
		return errors.Errorf("missing charm url in provision info")
	}

	charmInfo, err := a.facade.CharmInfo(provisionInfo.CharmURL.String())
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

	// TODO(embedded): container.Mounts[*].Path <= consolidate? => provisionInfo.Filesystems[*].Attachment.Path
	config := caas.ApplicationConfig{
		IntroductionSecret:   a.password,
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
		CharmModifiedVersion: provisionInfo.CharmModifiedVersion,
	}
	reason := "unchanged"
	// TODO(embedded): implement Equals method for caas.ApplicationConfig
	if !reflect.DeepEqual(config, a.lastApplied) {
		err = app.Ensure(config)
		if err != nil {
			return errors.Annotate(err, "ensuring application")
		}
		a.lastApplied = config
		reason = "deployed"
		if appState.Exists {
			reason = "updated"
		}
	}

	err = a.facade.SetOperatorStatus(a.name, status.Active, reason, nil)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (a *appWorker) dying(app caas.Application) error {
	a.logger.Debugf("application %q dying", a.name)
	err := app.Delete()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *appWorker) dead(app caas.Application) error {
	a.logger.Debugf("application %q dead", a.name)
	err := a.dying(app)
	if err != nil {
		return errors.Trace(err)
	}
	err = a.waitForTerminated(app)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = a.updateState(app, true, nil)
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
