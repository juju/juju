// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/utils/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
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

	name        string
	modelTag    names.ModelTag
	changes     chan struct{}
	password    string
	lastApplied caas.ApplicationConfig
}

type AppWorkerConfig struct {
	Name       string
	Facade     CAASProvisionerFacade
	Broker     CAASBroker
	ModelTag   names.ModelTag
	Clock      clock.Clock
	Logger     Logger
	UnitFacade CAASUnitProvisionerFacade
}

type NewAppWorkerFunc func(AppWorkerConfig) func() (worker.Worker, error)

func NewAppWorker(config AppWorkerConfig) func() (worker.Worker, error) {
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
	shouldExit, err := a.verifyCharmUpgraded()
	if err != nil {
		return errors.Trace(err)
	}
	if shouldExit {
		return nil
	}

	// If the application has an operator pod due to an upgrade-charm from a
	// pod-spec charm to a sidecar charm, delete it. Also delete workload pod.
	const maxDeleteLoops = 20
	for i := 0; ; i++ {
		if i >= maxDeleteLoops {
			return fmt.Errorf("couldn't delete operator and service with %d tries", maxDeleteLoops)
		}
		if i > 0 {
			select {
			case <-a.clock.After(3 * time.Second):
			case <-a.catacomb.Dying():
				return a.catacomb.ErrDying()
			}
		}

		exists, err := a.broker.OperatorExists(a.name)
		if err != nil {
			return errors.Trace(err)
		}
		if !exists.Exists {
			break
		}

		a.logger.Infof("deleting workload and operator pods for application %q", a.name)
		err = a.broker.DeleteService(a.name)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Annotatef(err, "deleting workload pod for application %q", a.name)
		}

		// Wait till the units are gone, to ensure worker code isn't messing
		// with old units, only new sidecar pods.
		const maxUnitsLoops = 20
		for j := 0; ; j++ {
			if j >= maxUnitsLoops {
				return fmt.Errorf("pods still present after %d tries", maxUnitsLoops)
			}
			units, err := a.broker.Units(a.name, caas.ModeWorkload)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Annotatef(err, "fetching workload units for application %q", a.name)
			}
			if len(units) == 0 {
				break
			}
			a.logger.Debugf("%q: waiting for workload pods to be deleted", a.name)
			select {
			case <-a.clock.After(3 * time.Second):
			case <-a.catacomb.Dying():
				return a.catacomb.ErrDying()
			}
		}

		err = a.broker.DeleteOperator(a.name)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Annotatef(err, "deleting operator pod for application %q", a.name)
		}
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

	// TODO(sidecar): support more than statefulset
	app := a.broker.Application(a.name, caas.DeploymentStateful)

	var appLife life.Value
	var appChanges watcher.NotifyChannel
	var appStateChanges watcher.NotifyChannel
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

	var scaleChan <-chan time.Time
	var scaleTries int
	var trustChan <-chan time.Time
	var trustTries int
	const maxRetries = 20
	const retryDelay = 3 * time.Second

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
			err := a.ensureScale(app)
			if errors.IsNotFound(err) {
				if scaleTries >= maxRetries {
					return errors.Annotatef(err, "more than %d retries ensuring scale", maxRetries)
				}
				scaleTries++
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
			err := a.ensureTrust(app)
			if errors.IsNotFound(err) {
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
		case <-a.catacomb.Dying():
			return a.catacomb.ErrDying()
		case <-appStateChanges:
			err = handleChange()
			if err != nil {
				return errors.Trace(err)
			}
		case <-a.changes:
			// Respond to life changes (Notify called by parent worker).
			err = handleChange()
			if err != nil {
				return errors.Trace(err)
			}
		case <-appChanges:
			// Respond to changes in provider application.
			lastReportedStatus, err = a.updateState(app, false, lastReportedStatus)
			if err != nil {
				return errors.Trace(err)
			}
		case <-replicaChanges:
			// Respond to changes in replicas of the application.
			lastReportedStatus, err = a.updateState(app, false, lastReportedStatus)
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
			if err = a.refreshApplicationStatus(app, appLife); err != nil {
				return errors.Annotatef(err, "refreshing application status for %q", a.name)
			}
		}
	}
}

func getTagsFromUnits(in []params.CAASUnit) []names.Tag {
	var out []names.Tag
	for _, v := range in {
		out = append(out, v.Tag)
	}
	return out
}

func (a *appWorker) charmFormat() (charm.Format, error) {
	charmInfo, err := a.facade.ApplicationCharmInfo(a.name)
	if err != nil {
		return charm.FormatUnknown, errors.Annotatef(err, "failed to get charm info for application %q", a.name)
	}
	return charm.MetaFormat(charmInfo.Charm()), nil
}

// verifyCharmUpgraded waits till the charm is upgraded to a v2 charm.
func (a *appWorker) verifyCharmUpgraded() (shouldExit bool, err error) {
	appStateWatcher, err := a.facade.WatchApplication(a.name)
	if err != nil {
		return false, errors.Annotatef(err, "failed to watch for changes to application %q", a.name)
	}
	if err := a.catacomb.Add(appStateWatcher); err != nil {
		return false, errors.Trace(err)
	}
	defer appStateWatcher.Kill()

	appStateChanges := appStateWatcher.Changes()
	for {
		format, err := a.charmFormat()
		if errors.IsNotFound(err) {
			a.logger.Debugf("application %q no longer exists", a.name)
			return true, nil
		} else if err != nil {
			return false, errors.Trace(err)
		}
		if format >= charm.FormatV2 {
			a.logger.Debugf("application %q is now a v2 charm", a.name)
			return false, nil
		}

		appLife, err := a.facade.Life(a.name)
		if errors.IsNotFound(err) {
			a.logger.Debugf("application %q no longer exists", a.name)
			return true, nil
		} else if err != nil {
			return false, errors.Trace(err)
		}
		if appLife == life.Dead {
			a.logger.Debugf("application %q now dead", a.name)
			return true, nil
		}

		// Wait for next app change, then loop to check charm format again.
		select {
		case <-appStateChanges:
		case <-a.catacomb.Dying():
			return false, a.catacomb.ErrDying()
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
	appTag := names.NewApplicationTag(a.name).String()
	appStatus := params.EntityStatus{}
	svc, err := app.Service()
	if errors.IsNotFound(err) {
		// Do nothing
	} else if err != nil {
		return nil, errors.Trace(err)
	} else {
		appStatus = params.EntityStatus{
			Status: svc.Status.Status,
			Info:   svc.Status.Message,
			Data:   svc.Status.Data,
		}
		err = a.unitFacade.UpdateApplicationService(params.UpdateApplicationServiceArg{
			ApplicationTag: appTag,
			ProviderId:     svc.Id,
			Addresses:      params.FromProviderAddresses(svc.Addresses...),
		})
		if errors.IsNotFound(err) {
			// Do nothing
		} else if err != nil {
			return nil, errors.Trace(err)
		}
	}

	err = a.facade.GarbageCollect(a.name, getTagsFromUnits(observedUnits), st.DesiredReplicas, st.Replicas, force)
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
		ApplicationTag: appTag,
		Status:         appStatus,
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
			err = a.broker.AnnotateUnit(a.name, caas.ModeSidecar, unitInfo.ProviderId, unit)
			if errors.IsNotFound(err) {
				continue
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}
	}
	return reportedStatus, nil
}

func (a *appWorker) refreshApplicationStatus(app caas.Application, appLife life.Value) error {
	if appLife != life.Alive {
		return nil
	}
	st, err := app.State()
	if errors.IsNotFound(err) {
		// Do nothing.
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// refresh the units information.
	units, err := a.facade.Units(a.name)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	readyUnitsCount := 0
	for _, unit := range units {
		if unit.UnitStatus.AgentStatus.Status == string(status.Active) {
			readyUnitsCount++
		}
	}
	if st.DesiredReplicas > 0 && st.DesiredReplicas > readyUnitsCount {
		// Only set status to waiting for scale up.
		// When the application gets scaled down, the desired units will be kept running and
		// the application should be active always.
		return a.setApplicationStatus(status.Waiting, "waiting for units settled down", nil)
	}
	return a.setApplicationStatus(status.Active, "", nil)
}

func (a *appWorker) ensureScale(app caas.Application) error {
	desiredScale, err := a.unitFacade.ApplicationScale(a.name)
	if err != nil {
		return errors.Annotatef(err, "fetching application %q desired scale", a.name)
	}

	a.logger.Debugf("updating application %q scale to %d", a.name, desiredScale)
	err = app.Scale(desiredScale)
	if err != nil {
		return errors.Annotatef(
			err,
			"scaling application %q to desired scale %d",
			a.name,
			desiredScale)
	}

	return nil
}

func (a *appWorker) ensureTrust(app caas.Application) error {
	desiredTrust, err := a.unitFacade.ApplicationTrust(a.name)
	if err != nil {
		return errors.Annotatef(err, "fetching application %q desired trust", a.name)
	}

	a.logger.Debugf("updating application %q trust to %v", a.name, desiredTrust)
	err = app.Trust(desiredTrust)
	if err != nil {
		return errors.Annotatef(
			err,
			"updating application %q to desired trust %v",
			a.name,
			desiredTrust)
	}

	return nil
}

func (a *appWorker) alive(app caas.Application) error {
	a.logger.Debugf("ensuring application %q exists", a.name)

	provisionInfo, err := a.facade.ProvisioningInfo(a.name)
	if err != nil {
		return errors.Annotate(err, "retrieving provisioning info")
	}
	if provisionInfo.CharmURL == nil {
		return errors.Errorf("missing charm url in provision info")
	}

	charmInfo, err := a.facade.CharmInfo(provisionInfo.CharmURL.String())
	if err != nil {
		return errors.Annotatef(err, "retrieving charm deployment info for %q", a.name)
	}

	appState, err := app.Exists()
	if err != nil {
		return errors.Annotatef(err, "retrieving application state for %q", a.name)
	}

	if appState.Exists && appState.Terminating {
		if err := a.waitForTerminated(app); err != nil {
			return errors.Annotatef(err, "%q was terminating and there was an error waiting for it to stop", a.name)
		}
	}

	images, err := a.facade.ApplicationOCIResources(a.name)
	if err != nil {
		return errors.Annotate(err, "getting OCI image resources")
	}

	os, err := series.GetOSFromSeries(provisionInfo.Series)
	if err != nil {
		return errors.Trace(err)
	}

	ver, err := series.SeriesVersion(provisionInfo.Series)
	if err != nil {
		return errors.Trace(err)
	}

	ch := charmInfo.Charm()
	charmBaseImage, err := podcfg.ImageForBase(provisionInfo.ImageRepo.Repository, charm.Base{
		Name: strings.ToLower(os.String()),
		Channel: charm.Channel{
			Track: ver,
			Risk:  charm.Stable,
		},
	})
	if err != nil {
		return errors.Annotate(err, "getting image for base")
	}

	containers := make(map[string]caas.ContainerConfig)
	for k, v := range ch.Meta().Containers {
		container := caas.ContainerConfig{
			Name: k,
		}
		if v.Resource == "" {
			return errors.NotValidf("empty container resource reference")
		}
		image, ok := images[v.Resource]
		if !ok {
			return errors.NotFoundf("referenced charm base image resource %s", v.Resource)
		}
		container.Image = image
		for _, m := range v.Mounts {
			container.Mounts = append(container.Mounts, caas.MountConfig{
				StorageName: m.Storage,
				Path:        m.Location,
			})
		}
		containers[k] = container
	}

	// TODO(sidecar): container.Mounts[*].Path <= consolidate? => provisionInfo.Filesystems[*].Attachment.Path
	config := caas.ApplicationConfig{
		IsPrivateImageRepo:   provisionInfo.ImageRepo.IsPrivate(),
		IntroductionSecret:   a.password,
		AgentVersion:         provisionInfo.Version,
		AgentImagePath:       provisionInfo.ImagePath,
		ControllerAddresses:  strings.Join(provisionInfo.APIAddresses, ","),
		ControllerCertBundle: provisionInfo.CACert,
		ResourceTags:         provisionInfo.Tags,
		Constraints:          provisionInfo.Constraints,
		Filesystems:          provisionInfo.Filesystems,
		Devices:              provisionInfo.Devices,
		CharmBaseImagePath:   charmBaseImage,
		Containers:           containers,
		CharmModifiedVersion: provisionInfo.CharmModifiedVersion,
		Trust:                provisionInfo.Trust,
		InitialScale:         provisionInfo.Scale,
	}
	reason := "unchanged"
	// TODO(sidecar): implement Equals method for caas.ApplicationConfig
	if !reflect.DeepEqual(config, a.lastApplied) {
		if err = app.Ensure(config); err != nil {
			_ = a.setApplicationStatus(status.Error, err.Error(), nil)
			return errors.Annotatef(err, "ensuring application %q", a.name)
		}
		a.lastApplied = config
		reason = "deployed"
		if appState.Exists {
			reason = "updated"
		}
	}
	a.logger.Debugf("application %q was %q", a.name, reason)
	return nil
}

func (a *appWorker) setApplicationStatus(s status.Status, reason string, data map[string]interface{}) error {
	a.logger.Tracef("updating application %q status to %q, %q, %v", a.name, s, reason, data)
	return a.facade.SetOperatorStatus(a.name, s, reason, data)
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
