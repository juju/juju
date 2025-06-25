// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machineservice "github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var ClassifyDetachedStorage = storagecommon.ClassifyDetachedStorage

// MachineManagerAPI provides access to the MachineManager API facade.
type MachineManagerAPI struct {
	modelUUID       coremodel.UUID
	st              Backend
	storageAccess   StorageInterface
	pool            Pool
	authorizer      Authorizer
	check           *common.BlockChecker
	resources       facade.Resources
	leadership      Leadership
	store           objectstore.ObjectStore
	controllerStore objectstore.ObjectStore
	clock           clock.Clock

	agentBinaryService      AgentBinaryService
	agentPasswordService    AgentPasswordService
	applicationService      ApplicationService
	cloudService            CloudService
	controllerConfigService ControllerConfigService
	controllerNodeService   ControllerNodeService
	keyUpdaterService       KeyUpdaterService
	machineService          MachineService
	statusService           StatusService
	modelConfigService      ModelConfigService
	networkService          NetworkService

	logger corelogger.Logger
}

// NewMachineManagerAPI creates a new server-side MachineManager API facade.
func NewMachineManagerAPI(
	modelUUID coremodel.UUID,
	backend Backend,
	store, controllerStore objectstore.ObjectStore,
	storageAccess StorageInterface,
	pool Pool,
	auth Authorizer,
	resources facade.Resources,
	leadership Leadership,
	logger corelogger.Logger,
	clock clock.Clock,
	services Services,
) *MachineManagerAPI {
	api := &MachineManagerAPI{
		modelUUID:       modelUUID,
		st:              backend,
		store:           store,
		controllerStore: controllerStore,
		pool:            pool,
		authorizer:      auth,
		check:           common.NewBlockChecker(services.BlockCommandService),
		resources:       resources,
		leadership:      leadership,
		storageAccess:   storageAccess,
		clock:           clock,
		logger:          logger,

		agentBinaryService:      services.AgentBinaryService,
		agentPasswordService:    services.AgentPasswordService,
		applicationService:      services.ApplicationService,
		controllerConfigService: services.ControllerConfigService,
		controllerNodeService:   services.ControllerNodeService,
		cloudService:            services.CloudService,
		keyUpdaterService:       services.KeyUpdaterService,
		machineService:          services.MachineService,
		statusService:           services.StatusService,
		modelConfigService:      services.ModelConfigService,
		networkService:          services.NetworkService,
	}
	return api
}

// AddMachines adds new machines with the supplied parameters.
// The args will contain Base info.
func (mm *MachineManagerAPI) AddMachines(ctx context.Context, args params.AddMachines) (params.AddMachinesResults, error) {
	results := params.AddMachinesResults{
		Machines: make([]params.AddMachinesResult, len(args.MachineParams)),
	}
	if err := mm.authorizer.CanWrite(ctx); err != nil {
		return results, err
	}
	if err := mm.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Trace(err)
	}

	allSpaces, err := mm.networkService.GetAllSpaces(ctx)
	if err != nil {
		return results, errors.Trace(err)
	}
	for i, p := range args.MachineParams {
		m, err := mm.addOneMachine(ctx, p, allSpaces)
		results.Machines[i].Error = apiservererrors.ServerError(err)
		if err == nil {
			results.Machines[i].Machine = m.Id()
		}
	}
	return results, nil
}

func (mm *MachineManagerAPI) addOneMachine(ctx context.Context, p params.AddMachineParams, allSpaces network.SpaceInfos) (result Machine, err error) {
	if p.ParentId != "" && p.ContainerType == "" {
		return nil, fmt.Errorf("parent machine specified without container type")
	}
	if p.ContainerType != "" && p.Placement != nil {
		return nil, fmt.Errorf("container type and placement are mutually exclusive")
	}
	if p.Placement != nil {
		// Extract container type and parent from container placement directives.
		containerType, err := instance.ParseContainerType(p.Placement.Scope)
		if err == nil {
			p.ContainerType = containerType
			p.ParentId = p.Placement.Directive
			p.Placement = nil
		}
	}

	if p.ContainerType != "" || p.Placement != nil {
		// Guard against dubious client by making sure that
		// the following attributes can only be set when we're
		// not using placement.
		p.InstanceId = ""
		p.Nonce = ""
		p.HardwareCharacteristics = instance.HardwareCharacteristics{}
		p.Addrs = nil
	}

	var base corebase.Base
	if p.Base == nil {
		conf, err := mm.modelConfigService.ModelConfig(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		base = config.PreferredBase(conf)
	} else {
		var err error
		base, err = corebase.ParseBase(p.Base.Name, p.Base.Channel)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	var placementDirective string
	if p.Placement != nil {
		if p.Placement.Scope != mm.modelUUID.String() {
			return nil, fmt.Errorf("invalid model id %q", p.Placement.Scope)
		}
		placementDirective = p.Placement.Directive
	}

	volumes := make([]state.HostVolumeParams, 0, len(p.Disks))
	for _, cons := range p.Disks {
		if cons.Count == 0 {
			return nil, errors.Errorf("invalid volume params: count not specified")
		}
		// Pool and Size are validated by AddMachineX.
		volumeParams := state.VolumeParams{
			Pool: cons.Pool,
			Size: cons.Size,
		}
		volumeAttachmentParams := state.VolumeAttachmentParams{}
		for i := uint64(0); i < cons.Count; i++ {
			volumes = append(volumes, state.HostVolumeParams{
				Volume: volumeParams, Attachment: volumeAttachmentParams,
			})
		}
	}

	// Convert the params to provider addresses, then convert those to
	// space addresses by looking up the spaces.
	pas := params.ToProviderAddresses(p.Addrs...)
	sAddrs, err := pas.ToSpaceAddresses(allSpaces)
	if err != nil {
		return nil, errors.Trace(err)
	}

	template := state.MachineTemplate{
		Base:                    state.Base{OS: base.OS, Channel: base.Channel.String()},
		Constraints:             p.Constraints,
		Volumes:                 volumes,
		InstanceId:              p.InstanceId,
		HardwareCharacteristics: p.HardwareCharacteristics,
		Addresses:               sAddrs,
		Placement:               placementDirective,
	}

	defer func() {
		if err == nil {
			// Ensure machine(s) exist in dqlite.
			err = mm.saveMachineInfo(ctx, p.Nonce)
		}
	}()

	if p.ContainerType == "" {
		return mm.st.AddOneMachine(template)
	}
	if p.ParentId != "" {
		return mm.st.AddMachineInsideMachine(template, p.ParentId, p.ContainerType)
	}
	return mm.st.AddMachineInsideNewMachine(template, template, p.ContainerType)
}

func (mm *MachineManagerAPI) saveMachineInfo(ctx context.Context, nonce string) error {
	// This is temporary - just insert the machine id all al the parent ones.
	var n *string
	if nonce != "" {
		n = &nonce
	}
	createMachineArgs := machineservice.CreateMachineArgs{
		Nonce: n,
	}
	_, _, err := mm.machineService.CreateMachine(ctx, createMachineArgs)
	return errors.Trace(err)
}

// ProvisioningScript returns a shell script that, when run,
// provisions a machine agent on the machine executing the script.
func (mm *MachineManagerAPI) ProvisioningScript(ctx context.Context, args params.ProvisioningScriptParams) (params.ProvisioningScriptResult, error) {
	if err := mm.authorizer.CanWrite(ctx); err != nil {
		return params.ProvisioningScriptResult{}, err
	}

	var result params.ProvisioningScriptResult
	st, err := mm.pool.SystemState()
	if err != nil {
		return result, errors.Trace(err)
	}

	services := InstanceConfigServices{
		CloudService:            mm.cloudService,
		ControllerConfigService: mm.controllerConfigService,
		ControllerNodeService:   mm.controllerNodeService,
		ObjectStore:             mm.controllerStore,
		KeyUpdaterService:       mm.keyUpdaterService,
		ModelConfigService:      mm.modelConfigService,
		MachineService:          mm.machineService,
		AgentBinaryService:      mm.agentBinaryService,
		AgentPasswordService:    mm.agentPasswordService,
	}

	icfg, err := InstanceConfig(
		ctx,
		mm.modelUUID,
		mm.machineService.GetBootstrapEnviron,
		st,
		mm.st, services, args.MachineId, args.Nonce, args.DataDir)
	if err != nil {
		return result, apiservererrors.ServerError(errors.Annotate(
			err, "getting instance config",
		))
	}

	// Until DisablePackageCommands is retired, for backwards
	// compatibility, we must respect the client's request and
	// override any model settings the user may have specified.
	// If the client does specify this setting, it will only ever be
	// true. False indicates the client doesn't care and we should use
	// what's specified in the environment config.
	if args.DisablePackageCommands {
		icfg.EnableOSRefreshUpdate = false
		icfg.EnableOSUpgrade = false
	} else {
		config, err := mm.modelConfigService.ModelConfig(ctx)
		if err != nil {
			mm.logger.Errorf(ctx,
				"cannot getting model config for provisioning machine %q: %v",
				args.MachineId, err,
			)
			return result, errors.New("controller failed to get model config for machine")
		}

		icfg.EnableOSUpgrade = config.EnableOSUpgrade()
		icfg.EnableOSRefreshUpdate = config.EnableOSRefreshUpdate()
	}

	getProvisioningScript := sshprovisioner.ProvisioningScript
	result.Script, err = getProvisioningScript(icfg)
	if err != nil {
		return result, apiservererrors.ServerError(errors.Annotate(
			err, "getting provisioning script",
		))
	}

	return result, nil
}

// RetryProvisioning marks a provisioning error as transient on the machines.
func (mm *MachineManagerAPI) RetryProvisioning(ctx context.Context, p params.RetryProvisioningArgs) (params.ErrorResults, error) {
	if err := mm.authorizer.CanWrite(ctx); err != nil {
		return params.ErrorResults{}, err
	}

	if err := mm.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{}
	machines, err := mm.st.AllMachines()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	wanted := set.NewStrings()
	for _, tagStr := range p.Machines {
		tag, err := names.ParseMachineTag(tagStr)
		if err != nil {
			result.Results = append(result.Results, params.ErrorResult{Error: apiservererrors.ServerError(err)})
			continue
		}
		wanted.Add(tag.Id())
	}
	for _, m := range machines {
		if !p.All && !wanted.Contains(m.Id()) {
			continue
		}
		machineName := coremachine.Name(m.Id())
		if err := mm.maybeUpdateInstanceStatus(ctx, p.All, machineName, map[string]interface{}{"transient": true}); err != nil {
			result.Results = append(result.Results, params.ErrorResult{Error: apiservererrors.ServerError(err)})
		}
	}
	return result, nil
}

func (mm *MachineManagerAPI) maybeUpdateInstanceStatus(ctx context.Context, all bool, machineName coremachine.Name, data map[string]interface{}) error {
	existingStatusInfo, err := mm.statusService.GetInstanceStatus(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return errors.NotFoundf("machine %q", machineName)
	} else if err != nil {
		return err
	}

	newData := existingStatusInfo.Data
	if newData == nil {
		newData = data
	} else {
		for k, v := range data {
			newData[k] = v
		}
	}
	if len(newData) > 0 && existingStatusInfo.Status != status.Error && existingStatusInfo.Status != status.ProvisioningError {
		// If a specifc machine has been asked for and it's not in error, that's a problem.
		if !all {
			return fmt.Errorf("machine %s is not in an error state (%v)", machineName, existingStatusInfo.Status)
		}
		// Otherwise just skip it.
		return nil
	}
	now := mm.clock.Now()
	sInfo := status.StatusInfo{
		Status:  existingStatusInfo.Status,
		Message: existingStatusInfo.Message,
		Data:    newData,
		Since:   &now,
	}
	err = mm.statusService.SetInstanceStatus(ctx, machineName, sInfo)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return errors.NotFoundf("machine %q", machineName)
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// DestroyMachineWithParams removes a set of machines from the model.
func (mm *MachineManagerAPI) DestroyMachineWithParams(ctx context.Context, args params.DestroyMachinesParams) (params.DestroyMachineResults, error) {
	entities := params.Entities{Entities: make([]params.Entity, len(args.MachineTags))}
	for i, tag := range args.MachineTags {
		entities.Entities[i].Tag = tag
	}
	return mm.destroyMachine(ctx, entities, args.Force, args.Keep, args.DryRun, common.MaxWait(args.MaxWait))
}

func (mm *MachineManagerAPI) destroyMachine(ctx context.Context, args params.Entities, force, keep, dryRun bool, maxWait time.Duration) (params.DestroyMachineResults, error) {
	if err := mm.authorizer.CanWrite(ctx); err != nil {
		return params.DestroyMachineResults{}, err
	}
	if err := mm.check.RemoveAllowed(ctx); err != nil {
		return params.DestroyMachineResults{}, err
	}
	results := make([]params.DestroyMachineResult, len(args.Entities))
	for i, entity := range args.Entities {
		result := params.DestroyMachineResult{}
		fail := func(e error) {
			result.Error = apiservererrors.ServerError(e)
			results[i] = result
		}

		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			fail(err)
			continue
		}
		machineName := coremachine.Name(machineTag.Id())

		if keep {
			mm.logger.Infof(ctx, "destroy machine %v but keep instance", machineName)
			if err := mm.machineService.SetKeepInstance(ctx, machineName, keep); err != nil {
				if !force {
					fail(err)
					continue
				}
				mm.logger.Warningf(ctx, "could not keep instance for machine %v: %v", machineName, err)
			}
		}
		info := params.DestroyMachineInfo{
			MachineId: machineTag.Id(),
		}

		machine, err := mm.st.Machine(machineTag.Id())
		if err != nil {
			fail(err)
			continue
		}
		containers, err := machine.Containers()
		if err != nil {
			fail(err)
			continue
		}
		if force || dryRun {
			info.DestroyedContainers, err = mm.destroyContainer(ctx, containers, force, keep, dryRun, maxWait)
			if err != nil {
				fail(err)
				continue
			}
		}

		unitNames, err := mm.applicationService.GetUnitNamesOnMachine(ctx, machineName)
		if errors.Is(err, applicationerrors.MachineNotFound) {
			fail(errors.NotFoundf("machine %s", machineName))
			continue
		} else if err != nil {
			fail(err)
			continue
		}

		for _, unitName := range unitNames {
			unitTag := names.NewUnitTag(unitName.String())
			info.DestroyedUnits = append(info.DestroyedUnits, params.Entity{Tag: unitTag.String()})
		}

		info.DestroyedStorage, info.DetachedStorage, err = mm.classifyDetachedStorage(unitNames)
		if err != nil {
			if !force {
				fail(err)
				continue
			}
			mm.logger.Warningf(ctx, "could not deal with units' storage on machine %v: %v", machineName, err)
		}

		if dryRun {
			result.Info = &info
			results[i] = result
			continue
		}

		applicationNames, err := mm.leadership.GetMachineApplicationNames(ctx, machineTag.Id())
		if err != nil {
			fail(err)
			continue
		}

		if force {
			if err := machine.ForceDestroy(maxWait); err != nil {
				fail(err)
				continue
			}
		} else {
			if err := machine.Destroy(mm.store); err != nil {
				fail(err)
				continue
			}
		}

		// Ensure that when the machine has been removed that all the leadership
		// references to that machine are also cleared up.
		//
		// Unfortunately we can't follow the normal practices of failing on the
		// error, as we've already removed the machine and we'll tell the caller
		// that we failed to remove the machine.
		//
		// Note: in some cases if a application has pinned during series upgrade
		// and it has been pinned without a timeout, then the leadership will
		// still prevent another leadership change. The work around for this
		// case until we provide the ability for the operator to unpin via the
		// CLI, is to remove the raft logs manually.
		unpinResults, err := mm.leadership.UnpinApplicationLeadersByName(ctx, machineTag, applicationNames)
		if err != nil {
			mm.logger.Warningf(ctx, "could not unpin application leaders for machine %s with error %v", machineTag.Id(), err)
		}
		for _, result := range unpinResults.Results {
			if result.Error != nil {
				mm.logger.Warningf(ctx,
					"could not unpin application leaders for machine %s with error %v", machineTag.Id(), result.Error)
			}
		}
		result.Info = &info
		results[i] = result
	}
	return params.DestroyMachineResults{Results: results}, nil
}

func (mm *MachineManagerAPI) destroyContainer(ctx context.Context, containers []string, force, keep, dryRun bool, maxWait time.Duration) ([]params.DestroyMachineResult, error) {
	if len(containers) == 0 {
		return nil, nil
	}
	entities := params.Entities{Entities: make([]params.Entity, len(containers))}
	for i, container := range containers {
		entities.Entities[i] = params.Entity{Tag: names.NewMachineTag(container).String()}
	}
	results, err := mm.destroyMachine(ctx, entities, force, keep, dryRun, maxWait)
	return results.Results, err
}

func (mm *MachineManagerAPI) classifyDetachedStorage(unitNames []coreunit.Name) (destroyed, detached []params.Entity, _ error) {
	var storageErrors []params.ErrorResult
	storageError := func(e error) {
		storageErrors = append(storageErrors, params.ErrorResult{Error: apiservererrors.ServerError(e)})
	}

	storageSeen := names.NewSet()
	for _, unitName := range unitNames {
		unitTag := names.NewUnitTag(unitName.String())
		storage, err := storagecommon.UnitStorage(mm.storageAccess, unitTag)
		if err != nil {
			storageError(errors.Annotatef(err, "getting storage for unit %v", unitName))
			continue
		}

		// Filter out storage we've already seen. Shared
		// storage may be attached to multiple units.
		var unseen []state.StorageInstance
		for _, storage := range storage {
			storageTag := storage.StorageTag()
			if storageSeen.Contains(storageTag) {
				continue
			}
			storageSeen.Add(storageTag)
			unseen = append(unseen, storage)
		}
		storage = unseen

		unitDestroyed, unitDetached, err := ClassifyDetachedStorage(
			mm.storageAccess.VolumeAccess(), mm.storageAccess.FilesystemAccess(), storage,
		)
		if err != nil {
			storageError(errors.Annotatef(err, "classifying storage for destruction for unit %v", unitName))
			continue
		}
		destroyed = append(destroyed, unitDestroyed...)
		detached = append(detached, unitDetached...)
	}
	err := params.ErrorResults{Results: storageErrors}.Combine()
	return destroyed, detached, err
}

// ModelAuthorizer defines if a given operation can be performed based on a
// model tag.
type ModelAuthorizer struct {
	ModelTag   names.ModelTag
	Authorizer facade.Authorizer
}

// CanRead checks to see if a read is possible. Returns an error if a read is
// not possible.
func (a ModelAuthorizer) CanRead(ctx context.Context) error {
	return a.checkAccess(ctx, permission.ReadAccess)
}

// CanWrite checks to see if a write is possible. Returns an error if a write
// is not possible.
func (a ModelAuthorizer) CanWrite(ctx context.Context) error {
	return a.checkAccess(ctx, permission.WriteAccess)
}

// AuthClient returns true if the entity is an external user.
func (a ModelAuthorizer) AuthClient() bool {
	return a.Authorizer.AuthClient()
}

func (a ModelAuthorizer) checkAccess(ctx context.Context, access permission.Access) error {
	return a.Authorizer.HasPermission(ctx, access, a.ModelTag)
}
