// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	internalstorage "github.com/juju/juju/internal/storage"
)

// UpdateStatusState is used by UpdateState to not refresh known state.
type UpdateStatusState map[coreunit.Name]applicationservice.UpdateCAASUnitParams

// ProvisioningInfo holds all the information required to create the application
// in kubernetes.
type ProvisioningInfo struct {
	Version              semversion.Number
	APIAddresses         []string
	CACert               string
	Tags                 map[string]string
	Constraints          constraints.Value
	Devices              []devices.KubernetesDeviceParams
	Base                 base.Base
	ImageDetails         coreresource.DockerImageDetails
	CharmModifiedVersion int
	Trust                bool
	Scale                int

	CharmMeta           *charm.Meta
	Images              map[string]coreresource.DockerImageDetails
	FilesystemTemplates []storageprovisioning.FilesystemTemplate
	StorageResourceTags map[string]string
}

// ApplicationOps defines all the operations the application worker can perform.
// This is exported for testing only.
type ApplicationOps interface {
	ProvisioningInfo(
		ctx context.Context, appName string, appID coreapplication.ID,
		facade CAASProvisionerFacade,
		storageProvisioningService StorageProvisioningService,
		applicationService ApplicationService,
		resourceOpenerGetter ResourceOpenerGetter,
		lastProvisioningInfo *ProvisioningInfo,
		logger logger.Logger) (*ProvisioningInfo, error)

	AppAlive(ctx context.Context, appName string, app caas.Application,
		password string, lastApplied *caas.ApplicationConfig,
		provisioningInfo *ProvisioningInfo,
		statusService StatusService,
		clk clock.Clock, logger logger.Logger) error

	AppDying(ctx context.Context, appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
		facade CAASProvisionerFacade,
		applicationService ApplicationService, statusService StatusService,
		logger logger.Logger) error

	AppDead(ctx context.Context, appName string, appID coreapplication.ID, app caas.Application,
		broker CAASBroker, applicationService ApplicationService, statusService StatusService,
		clk clock.Clock, logger logger.Logger) error

	EnsureTrust(ctx context.Context, appName string, app caas.Application,
		applicationService ApplicationService, logger logger.Logger) error

	UpdateState(ctx context.Context, appName string, appID coreapplication.ID, app caas.Application,
		lastReportedStatus UpdateStatusState,
		broker CAASBroker, applicationService ApplicationService, statusService StatusService,
		clk clock.Clock, logger logger.Logger) (UpdateStatusState, error)

	RefreshApplicationStatus(ctx context.Context, appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
		statusService StatusService, clk clock.Clock, logger logger.Logger) error

	WaitForTerminated(appName string, app caas.Application,
		clk clock.Clock) error

	ReconcileDeadUnitScale(ctx context.Context, appName string, appID coreapplication.ID, app caas.Application,
		facade CAASProvisionerFacade,
		applicationService ApplicationService, statusService StatusService,
		logger logger.Logger) error

	EnsureScale(ctx context.Context, appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
		facade CAASProvisionerFacade,
		applicationService ApplicationService, statusService StatusService,
		logger logger.Logger) error
}

type applicationOps struct{}

var _ ApplicationOps = &applicationOps{}

func (applicationOps) ProvisioningInfo(
	ctx context.Context, appName string, appID coreapplication.ID,
	facade CAASProvisionerFacade,
	storageProvisioningService StorageProvisioningService,
	applicationService ApplicationService,
	resourceOpenerGetter ResourceOpenerGetter,
	lastProvisioningInfo *ProvisioningInfo,
	logger logger.Logger) (*ProvisioningInfo, error) {
	return provisioningInfo(ctx, appName, appID, facade, storageProvisioningService, applicationService, resourceOpenerGetter, lastProvisioningInfo, logger)
}

func (applicationOps) AppAlive(
	ctx context.Context,
	appName string, app caas.Application, password string,
	lastApplied *caas.ApplicationConfig,
	provisioningInfo *ProvisioningInfo,
	statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) error {
	return appAlive(ctx, appName, app, password, lastApplied, provisioningInfo, statusService, clk, logger)
}

func (applicationOps) AppDying(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
	facade CAASProvisionerFacade,
	applicationService ApplicationService, statusService StatusService,
	logger logger.Logger,
) error {
	return appDying(ctx, appName, appID, app, appLife, facade, applicationService, statusService, logger)
}

func (applicationOps) AppDead(ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, broker CAASBroker,
	applicationService ApplicationService, statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) error {
	return appDead(ctx, appName, appID, app, broker, applicationService, statusService, clk, logger)
}

func (applicationOps) EnsureTrust(
	ctx context.Context,
	appName string, app caas.Application,
	applicationService ApplicationService,
	logger logger.Logger,
) error {
	return ensureTrust(ctx, appName, app, applicationService, logger)
}

func (applicationOps) UpdateState(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, lastReportedStatus UpdateStatusState,
	broker CAASBroker, applicationService ApplicationService, statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) (UpdateStatusState, error) {
	return updateState(ctx, appName, appID, app, lastReportedStatus, broker, applicationService, statusService, clk, logger)
}

func (applicationOps) RefreshApplicationStatus(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
	statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) error {
	return refreshApplicationStatus(ctx, appName, appID, app, appLife, statusService, clk, logger)
}

func (applicationOps) WaitForTerminated(
	appName string, app caas.Application,
	clk clock.Clock,
) error {
	return waitForTerminated(appName, app, clk)
}

func (applicationOps) ReconcileDeadUnitScale(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application,
	facade CAASProvisionerFacade,
	applicationService ApplicationService, statusService StatusService,
	logger logger.Logger,
) error {
	return reconcileDeadUnitScale(ctx, appName, appID, app, facade, applicationService, statusService, logger)
}

func (applicationOps) EnsureScale(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
	facade CAASProvisionerFacade,
	applicationService ApplicationService, statusService StatusService,
	logger logger.Logger,
) error {
	return ensureScale(ctx, appName, appID, app, appLife, facade, applicationService, statusService, logger)
}

type Tomb interface {
	Dying() <-chan struct{}
	ErrDying() error
}

// appAlive handles the life.Alive state for the CAAS application. It handles invoking the
// CAAS broker to create the resources in the k8s cluster for this application.
func appAlive(ctx context.Context, appName string, app caas.Application,
	password string, lastApplied *caas.ApplicationConfig,
	pi *ProvisioningInfo,
	statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) error {
	logger.Debugf(ctx, "ensuring application %q exists", appName)

	appState, err := app.Exists()
	if err != nil {
		return errors.Annotatef(err, "retrieving application state for %q", appName)
	}

	if appState.Exists && appState.Terminating {
		if err := waitForTerminated(appName, app, clk); err != nil {
			return errors.Annotatef(err, "%q was terminating and there was an error waiting for it to stop", appName)
		}
	}

	charmBaseImage, err := podcfg.ImageForBase(pi.ImageDetails.Repository, charm.Base{
		Name: pi.Base.OS,
		Channel: charm.Channel{
			Track: pi.Base.Channel.Track,
			Risk:  charm.Risk(pi.Base.Channel.Risk),
		},
	})
	if err != nil {
		return errors.Annotate(err, "getting image for base")
	}

	containers := make(map[string]caas.ContainerConfig)
	for k, v := range pi.CharmMeta.Containers {
		container := caas.ContainerConfig{
			Name: k,
			Uid:  v.Uid,
			Gid:  v.Gid,
		}
		if v.Resource == "" {
			return errors.NotValidf("empty container resource reference")
		}
		image, ok := pi.Images[v.Resource]
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

	filesystems := []internalstorage.KubernetesFilesystemParams{}
	for _, fst := range pi.FilesystemTemplates {
		for i := range fst.Count {
			mountPoint, err := storage.FilesystemMountPointK8s(
				fst.Location, fst.MaxCount, i, fst.StorageName,
			)
			if err != nil {
				return errors.Trace(err)
			}
			fsp := internalstorage.KubernetesFilesystemParams{
				StorageName: fst.StorageName,
				Size:        fst.SizeMiB,
				Provider:    internalstorage.ProviderType(fst.ProviderType),
				Attributes: transform.Map(fst.Attributes, func(k, v string) (string, any) {
					return k, v
				}),
				Attachment: &internalstorage.KubernetesFilesystemAttachmentParams{
					ReadOnly: fst.ReadOnly,
					Path:     mountPoint,
				},
				ResourceTags: pi.StorageResourceTags,
			}
			filesystems = append(filesystems, fsp)
		}
	}

	// TODO(sidecar): container.Mounts[*].Path <= consolidate? => provisionInfo.Filesystems[*].Attachment.Path
	config := caas.ApplicationConfig{
		IsPrivateImageRepo:   pi.ImageDetails.IsPrivate(),
		IntroductionSecret:   password,
		AgentVersion:         pi.Version,
		AgentImagePath:       pi.ImageDetails.RegistryPath,
		ControllerAddresses:  strings.Join(pi.APIAddresses, ","),
		ControllerCertBundle: pi.CACert,
		ResourceTags:         pi.Tags,
		Constraints:          pi.Constraints,
		Filesystems:          filesystems,
		Devices:              pi.Devices,
		CharmBaseImagePath:   charmBaseImage,
		Containers:           containers,
		CharmModifiedVersion: pi.CharmModifiedVersion,
		Trust:                pi.Trust,
		InitialScale:         pi.Scale,
	}
	switch pi.CharmMeta.CharmUser {
	case charm.RunAsDefault:
		config.CharmUser = caas.RunAsDefault
	case charm.RunAsRoot:
		config.CharmUser = caas.RunAsRoot
	case charm.RunAsSudoer:
		config.CharmUser = caas.RunAsSudoer
	case charm.RunAsNonRoot:
		config.CharmUser = caas.RunAsNonRoot
	default:
		return errors.NotValidf("unknown RunAs for CharmUser: %q", pi.CharmMeta.CharmUser)
	}
	reason := "unchanged"
	// TODO(sidecar): implement Equals method for caas.ApplicationConfig
	if !reflect.DeepEqual(config, *lastApplied) {
		if err = app.Ensure(config); err != nil {
			_ = setApplicationStatus(ctx, appName, status.Error, err.Error(), nil, statusService, clk, logger)
			return errors.Annotatef(err, "ensuring application %q", appName)
		}
		*lastApplied = config
		reason = "deployed"
		if appState.Exists {
			reason = "updated"
		}
	}
	logger.Debugf(ctx, "application %q was %q", appName, reason)
	return nil
}

// appDying handles the life.Dying state for the CAAS application. It deals with scaling down
// the application and removing units.
func appDying(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
	facade CAASProvisionerFacade,
	applicationService ApplicationService, statusService StatusService,
	logger logger.Logger,
) (err error) {
	logger.Debugf(ctx, "application %q dying", appName)
	err = ensureScale(ctx, appName, appID, app, appLife, facade, applicationService, statusService, logger)
	if err != nil {
		return errors.Annotate(err, "cannot scale dying application to 0")
	}
	err = reconcileDeadUnitScale(ctx, appName, appID, app, facade, applicationService, statusService, logger)
	if err != nil {
		return errors.Annotate(err, "cannot reconcile dead units in dying application")
	}
	return nil
}

// appDead handles the life.Dead state for the CAAS application. It ensures the application
// is removed from the k8s cluster and unblocks the cleanup of the application in state.
func appDead(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, broker CAASBroker,
	applicationService ApplicationService, statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) error {
	logger.Debugf(ctx, "application %q dead", appName)
	err := app.Delete()
	if err != nil {
		return errors.Trace(err)
	}
	err = waitForTerminated(appName, app, clk)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = updateState(ctx, appName, appID, app, nil, broker, applicationService, statusService, clk, logger)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(k8s): re-implement this to prevent a dead app from going away through
	// creating a new domain concept that holds the application until this worker
	// has destroyed all the k8s resources.
	//
	// Clear "has-resources" flag so state knows it can now remove the application.
	return nil
}

// ensureTrust updates the applications Trust status on the CAAS broker, giving it
// access to the k8s api via a service account.
func ensureTrust(
	ctx context.Context,
	appName string, app caas.Application,
	applicationService ApplicationService,
	logger logger.Logger,
) error {
	desiredTrust, err := applicationService.GetApplicationTrustSetting(ctx, appName)
	if err != nil {
		return errors.Annotatef(err, "fetching application %q desired trust", appName)
	}

	logger.Debugf(ctx, "updating application %q trust to %v", appName, desiredTrust)
	err = app.Trust(desiredTrust)
	if err != nil {
		return errors.Annotatef(
			err,
			"updating application %q to desired trust %v",
			appName,
			desiredTrust)
	}
	return nil
}

// updateState reports back information about the CAAS application into state, such as
// status, IP addresses and volume info.
func updateState(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application,
	lastReportedStatus UpdateStatusState,
	broker CAASBroker, applicationService ApplicationService, statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) (UpdateStatusState, error) {
	svc, err := app.Service()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if svc != nil {
		err := applicationService.UpdateCloudService(
			ctx, appName, svc.Id, svc.Addresses)
		if err != nil && !errors.Is(err, applicationerrors.ApplicationNotFound) {
			return nil, errors.Trace(err)
		}
		now := clk.Now()
		err = statusService.SetApplicationStatus(ctx, appName, status.StatusInfo{
			Status:  svc.Status.Status,
			Message: svc.Status.Message,
			Data:    svc.Status.Data,
			Since:   &now,
		})
		if err != nil && !errors.Is(err, statuserrors.ApplicationNotFound) {
			return nil, errors.Trace(err)
		}
	}

	unitToPod, err := applicationService.GetAllUnitCloudContainerIDsForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	podToUnit := make(map[string]coreunit.Name, len(unitToPod))
	for k, v := range unitToPod {
		podToUnit[v] = k
	}

	units, err := app.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}

	reportedStatus := make(UpdateStatusState, len(units))
	for _, u := range units {
		unitName, ok := podToUnit[u.Id]
		if !ok {
			// This pod exists outside of Juju's knowledge. Ignore it for now.
			continue
		}

		args := applicationservice.UpdateCAASUnitParams{
			ProviderID: &u.Id,
			Address:    &u.Address,
			Ports:      &u.Ports,
		}
		args.AgentStatus, args.CloudContainerStatus = updateStatus(u.Status)

		lastStatus, ok := lastReportedStatus[unitName]
		reportedStatus[unitName] = args
		if ok {
			if reflect.DeepEqual(lastStatus, args) {
				// We've already reported this.
				continue
			}
		}

		err = applicationService.UpdateCAASUnit(ctx, unitName, args)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	for unitName, podName := range unitToPod {
		lastReported := lastReportedStatus[unitName]
		if lastReported.ProviderID != nil &&
			*lastReported.ProviderID == podName {
			// The pod has already been annotated.
			continue
		}

		unitTag := names.NewUnitTag(unitName.String())
		err := broker.AnnotateUnit(ctx, appName, podName, unitTag)
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return reportedStatus, nil
}

func refreshApplicationStatus(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
	statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) error {
	if appLife != life.Alive {
		return nil
	}
	st, err := app.State()
	if errors.Is(err, errors.NotFound) {
		// Do nothing.
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// refresh the unit's information.
	unitStatuses, err := statusService.GetUnitAgentStatusesForApplication(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	readyUnitsCount := 0
	for _, unit := range unitStatuses {
		switch unit.Status {
		case status.Idle, status.Executing:
			readyUnitsCount++
		}
	}
	if st.DesiredReplicas > 0 && st.DesiredReplicas > readyUnitsCount {
		// Only set status to waiting for scale up.
		// When the application gets scaled down, the desired units will be kept running and
		// the application should be active always.
		return setApplicationStatus(ctx, appName, status.Waiting, "waiting for units to settle down", nil, statusService, clk, logger)
	}
	return setApplicationStatus(ctx, appName, status.Active, "", nil, statusService, clk, logger)
}

func waitForTerminated(appName string, app caas.Application,
	clk clock.Clock) error {
	existsFunc := func() error {
		appState, err := app.Exists()
		if err != nil {
			return errors.Trace(err)
		}
		if !appState.Exists {
			return nil
		}
		if appState.Exists && !appState.Terminating {
			return errors.Errorf("application %q should be terminating but is now running", appName)
		}
		return tryAgain
	}
	retryCallArgs := retry.CallArgs{
		Attempts:    60,
		Delay:       3 * time.Second,
		MaxDuration: 3 * time.Minute,
		Clock:       clk,
		Func:        existsFunc,
		IsFatalError: func(err error) bool {
			return !errors.Is(err, tryAgain)
		},
	}
	return errors.Trace(retry.Call(retryCallArgs))
}

// reconcileDeadUnitScale is setup to respond to CAAS sidecar units that become
// dead. It takes stock of what the current desired scale is for the application
// and the number of dead units in the application. Once the number of dead units
// has reached the point where the desired scale has been achieved this func
// can go ahead and remove the units from CAAS provider.
func reconcileDeadUnitScale(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application,
	facade CAASProvisionerFacade,
	applicationService ApplicationService,
	statusService StatusService,
	logger logger.Logger,
) error {
	unitNamesAndLives, err := applicationService.GetAllUnitLifeForApplication(ctx, appID)
	if err != nil {
		return fmt.Errorf("getting units for application %s: %w", appName, err)
	}

	ps, err := applicationService.GetApplicationScalingState(ctx, appName)
	if err != nil {
		return errors.Trace(err)
	}
	if !ps.Scaling {
		return nil
	}

	desiredScale := ps.ScaleTarget
	unitsToRemove := 0

	var deadUnits []coreunit.Name
	for unitName, unitLife := range unitNamesAndLives {
		if unitName.Number() < desiredScale {
			// This is a unit we want to keep.
			continue
		}
		unitsToRemove++
		if unitLife == life.Dead {
			deadUnits = append(deadUnits, unitName)
		}
	}

	// We haven't met the threshold to initiate scale down in the CAAS provider
	// yet.
	if unitsToRemove != len(deadUnits) {
		return nil
	}

	logger.Infof(ctx, "scaling application %q to desired scale %d", appName, desiredScale)
	if err := app.Scale(desiredScale); err != nil && !errors.Is(err, errors.NotFound) {
		return fmt.Errorf(
			"scaling application %q to scale %d: %w",
			appName,
			desiredScale,
			err,
		)
	}

	appState, err := app.State()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return err
	}
	// TODO: stop k8s things from mutating the statefulset.
	if len(appState.Replicas) > desiredScale {
		return tryAgain
	}

	for _, deadUnit := range deadUnits {
		logger.Infof(ctx, "removing dead unit %s", deadUnit)
		if err := facade.RemoveUnit(ctx, string(deadUnit)); err != nil && !errors.Is(err, errors.NotFound) {
			return fmt.Errorf("removing dead unit %q: %w", deadUnit, err)
		}
	}

	return updateProvisioningState(ctx, appName, false, 0, applicationService)
}

// ensureScale determines how and when to scale up or down based on
// current scale targets that have yet to be met.
func ensureScale(
	ctx context.Context,
	appName string, appID coreapplication.ID, app caas.Application, appLife life.Value,
	facade CAASProvisionerFacade,
	applicationService ApplicationService, statusService StatusService,
	logger logger.Logger,
) error {
	var err error
	var desiredScale int
	switch appLife {
	case life.Alive:
		desiredScale, err = applicationService.GetApplicationScale(ctx, appName)
		if err != nil {
			return errors.Annotatef(err, "fetching application %q desired scale", appName)
		}
	case life.Dying, life.Dead:
		desiredScale = 0
	default:
		return errors.NotImplementedf("unknown life %q", appLife)
	}

	ps, err := applicationService.GetApplicationScalingState(ctx, appName)
	if err != nil {
		return errors.Trace(err)
	}

	logger.Debugf(ctx, "updating application %q scale to %d", appName, desiredScale)
	if !ps.Scaling || appLife != life.Alive {
		err := updateProvisioningState(ctx, appName, true, desiredScale, applicationService)
		if err != nil {
			return err
		}
		ps.Scaling = true
		ps.ScaleTarget = desiredScale
	}

	units, err := applicationService.GetAllUnitLifeForApplication(ctx, appID)
	if err != nil {
		return err
	}
	unitScale := 0
	for unitName := range units {
		nextUnitNumber := unitName.Number() + 1
		if nextUnitNumber > unitScale {
			unitScale = nextUnitNumber
		}
	}
	if ps.ScaleTarget >= unitScale {
		logger.Infof(ctx, "scaling application %q to desired scale %d", appName, ps.ScaleTarget)
		err = app.Scale(ps.ScaleTarget)
		if appLife != life.Alive && errors.Is(err, errors.NotFound) {
			logger.Infof(ctx, "dying application %q is already removed from k8s", appName)
			return updateProvisioningState(ctx, appName, false, 0, applicationService)
		} else if err != nil {
			return err
		}
		if ps.ScaleTarget > len(units) {
			// Scaling up must see units created.
			return tryAgain
		}
		err := updateProvisioningState(ctx, appName, false, 0, applicationService)
		if err != nil {
			return err
		}
		if ps.ScaleTarget != desiredScale {
			// if the current scale target doesn't equal the desired scale
			// we need to rerun this.
			logger.Debugf(ctx, "application %q currently scaling to %d but desired scale is %d", appName, ps.ScaleTarget, desiredScale)
			return tryAgain
		}
		return nil
	}

	var unitsToDestroy []string
	for unitName, unitLife := range units {
		if unitName.Number() < ps.ScaleTarget {
			// This is a unit we want to keep.
			continue
		}
		if unitLife == life.Alive {
			unitsToDestroy = append(unitsToDestroy, unitName.String())
		}
	}
	if len(unitsToDestroy) > 0 {
		if err := facade.DestroyUnits(ctx, unitsToDestroy); err != nil {
			return errors.Trace(err)
		}
	}

	if ps.ScaleTarget != desiredScale {
		// if the current scale target doesn't equal the desired scale
		// we need to rerun this.
		logger.Debugf(ctx, "application %q currently scaling to %d but desired scale is %d", appName, ps.ScaleTarget, desiredScale)
		return tryAgain
	}

	return nil
}

func setApplicationStatus(
	ctx context.Context,
	appName string, s status.Status, reason string, data map[string]any,
	statusService StatusService,
	clk clock.Clock, logger logger.Logger,
) error {
	logger.Tracef(ctx, "updating application %q status to %q, %q, %v", appName, s, reason, data)
	now := clk.Now()
	return statusService.SetApplicationStatus(ctx, appName, status.StatusInfo{
		Status:  s,
		Message: reason,
		Data:    data,
		Since:   &now,
	})
}

func updateProvisioningState(
	ctx context.Context,
	appName string, scaling bool, scaleTarget int,
	applicationService ApplicationService,
) error {
	err := applicationService.SetApplicationScalingState(ctx, appName, scaleTarget, scaling)
	if errors.Is(err, applicationerrors.ScalingStateInconsistent) {
		return tryAgain
	} else if err != nil {
		return errors.Annotatef(err, "setting provisiong state for application %q", appName)
	}
	return nil
}

func provisioningInfo(
	ctx context.Context,
	appName string, appID coreapplication.ID,
	facade CAASProvisionerFacade,
	storageProvisioningService StorageProvisioningService,
	applicationService ApplicationService,
	resourceOpenerGetter ResourceOpenerGetter,
	lastProvisioningInfo *ProvisioningInfo,
	logger logger.Logger,
) (*ProvisioningInfo, error) {
	// TODO(k8s): stop calling onto the facade to get these.
	res, err := facade.ProvisioningInfo(ctx, appName)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving provisioning info")
	}

	pi := &ProvisioningInfo{
		Version:              res.Version,
		APIAddresses:         res.APIAddresses,
		CACert:               res.CACert,
		Tags:                 res.Tags,
		Constraints:          res.Constraints,
		Devices:              res.Devices,
		Base:                 res.Base,
		ImageDetails:         res.ImageDetails,
		CharmModifiedVersion: res.CharmModifiedVersion,
		Trust:                res.Trust,
		Scale:                res.Scale,
	}

	fsTemplates, err := storageProvisioningService.GetFilesystemTemplatesForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pi.FilesystemTemplates = fsTemplates

	storageResourceTags, err := storageProvisioningService.GetStorageResourceTagsForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pi.StorageResourceTags = storageResourceTags

	if lastProvisioningInfo != nil {
		if pi.CharmModifiedVersion == lastProvisioningInfo.CharmModifiedVersion {
			pi.CharmMeta = lastProvisioningInfo.CharmMeta
			pi.Images = lastProvisioningInfo.Images
			return pi, nil
		}
	}

	charm, _, err := applicationService.GetCharmByApplicationID(ctx, appID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pi.CharmMeta = charm.Meta()

	ro, err := resourceOpenerGetter.ResourceOpenerForApplication(ctx, appID, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	pi.Images = make(map[string]coreresource.DockerImageDetails)
	for _, v := range charm.Meta().Resources {
		if v.Type != charmresource.TypeContainerImage {
			continue
		}
		opened, err := ro.OpenResource(ctx, v.Name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		rsc, err := readDockerImageResource(opened)
		_ = opened.Close()
		if err != nil {
			return nil, errors.Trace(err)
		}
		pi.Images[v.Name] = rsc
		err = ro.SetResourceUsed(ctx, opened.UUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return pi, nil
}

// updateStatus constructs the agent and cloud container status values.
func updateStatus(podStatus status.StatusInfo) (
	agentStatus *status.StatusInfo,
	cloudContainerStatus *status.StatusInfo,
) {
	switch podStatus.Status {
	case status.Unknown:
		// The container runtime can spam us with unimportant
		// status updates, so ignore any irrelevant ones.
		return nil, nil
	case status.Allocating:
		// The container runtime has decided to restart the pod.
		agentStatus = &status.StatusInfo{
			Status:  status.Allocating,
			Message: podStatus.Message,
		}
		cloudContainerStatus = &status.StatusInfo{
			Status:  status.Waiting,
			Message: podStatus.Message,
			Data:    podStatus.Data,
		}
	case status.Running:
		// A pod has finished starting so the workload is now active.
		agentStatus = &status.StatusInfo{
			Status: status.Idle,
		}
		cloudContainerStatus = &status.StatusInfo{
			Status:  status.Running,
			Message: podStatus.Message,
			Data:    podStatus.Data,
		}
	case status.Error:
		agentStatus = &status.StatusInfo{
			Status:  status.Error,
			Message: podStatus.Message,
			Data:    podStatus.Data,
		}
		cloudContainerStatus = &status.StatusInfo{
			Status:  status.Error,
			Message: podStatus.Message,
			Data:    podStatus.Data,
		}
	case status.Blocked:
		agentStatus = &status.StatusInfo{
			Status: status.Idle,
		}
		cloudContainerStatus = &status.StatusInfo{
			Status:  status.Blocked,
			Message: podStatus.Message,
			Data:    podStatus.Data,
		}
	}
	return agentStatus, cloudContainerStatus
}

func readDockerImageResource(reader io.Reader) (coreresource.DockerImageDetails, error) {
	contents, err := io.ReadAll(reader)
	if err != nil {
		return coreresource.DockerImageDetails{}, errors.Trace(err)
	}
	var details docker.DockerImageDetails
	if err := json.Unmarshal(contents, &details); err != nil {
		if err := yaml.Unmarshal(contents, &details); err != nil {
			return coreresource.DockerImageDetails{}, errors.Annotate(err, "file neither valid json or yaml")
		}
	}
	if err := docker.ValidateDockerRegistryPath(details.RegistryPath); err != nil {
		return coreresource.DockerImageDetails{}, err
	}
	return coreresource.DockerImageDetails{
		RegistryPath:     details.RegistryPath,
		ImageRepoDetails: docker.ConvertToResourceImageDetails(details.ImageRepoDetails),
	}, nil
}
