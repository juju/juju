// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/blockcommand"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var ClassifyDetachedStorage = storagecommon.ClassifyDetachedStorage

// ControllerConfigService defines a method for getting the controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// AgentFinderService defines a method for finding agent binary metadata.
type AgentFinderService interface {
	FindAgents(context.Context, modelagent.FindAgentsParams) (coretools.List, error)
}

// KeyUpdaterService is responsible for returning information about the ssh keys
// for a machine within a model.
type KeyUpdaterService interface {
	// GetAuthorisedKeysForMachine returns the authorized keys that should be
	// allowed to access the given machine.
	GetAuthorisedKeysForMachine(context.Context, coremachine.Name) ([]string, error)
}

// ModelConfigService is responsible for providing an accessor to the models
// config.
type ModelConfigService interface {
	// ModelConfig provides the currently set model config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// Leadership represents a type for modifying the leadership settings of an
// application for series upgrades.
type Leadership interface {
	// GetMachineApplicationNames returns the applications associated with a
	// machine.
	GetMachineApplicationNames(context.Context, string) ([]string, error)

	// UnpinApplicationLeadersByName takes a slice of application names and
	// attempts to unpin them accordingly.
	UnpinApplicationLeadersByName(context.Context, names.Tag, []string) (params.PinApplicationsResults, error)
}

// Authorizer checks to see if an operation can be performed.
type Authorizer interface {
	// CanRead checks to see if a read is possible. Returns an error if a read
	// is not possible.
	CanRead(context.Context) error

	// CanWrite checks to see if a write is possible. Returns an error if a
	// write is not possible.
	CanWrite(context.Context) error

	// AuthClient returns true if the entity is an external user.
	AuthClient() bool
}

// MachineService is the interface that is used to interact with the machines.
type MachineService interface {
	// CreateMachine creates a machine with the given name.
	CreateMachine(context.Context, coremachine.Name) (string, error)
	// DeleteMachine deletes a machine with the given name.
	DeleteMachine(context.Context, coremachine.Name) error
	// GetBootstrapEnviron returns the bootstrap environ.
	GetBootstrapEnviron(context.Context) (environs.BootstrapEnviron, error)
	// GetInstanceTypesFetcher returns the instance types fetcher.
	GetInstanceTypesFetcher(context.Context) (environs.InstanceTypesFetcher, error)
	// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
	// the corresponding cloud instance to be stopped.
	// It returns a NotFound if the given machine doesn't exist.
	ShouldKeepInstance(ctx context.Context, machineName coremachine.Name) (bool, error)
	// SetKeepInstance sets whether the machine cloud instance will be retained
	// when the machine is removed from Juju. This is only relevant if an instance
	// exists.
	// It returns a NotFound if the given machine doesn't exist.
	SetKeepInstance(ctx context.Context, machineName coremachine.Name, keep bool) error
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (string, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID string) (*instance.HardwareCharacteristics, error)
}

// CharmhubClient represents a way for querying the charmhub api for information
// about the application charm.
type CharmhubClient interface {
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
}

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently in place.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
}

// MachineManagerAPI provides access to the MachineManager API facade.
type MachineManagerAPI struct {
	model                   coremodel.ModelInfo
	controllerConfigService ControllerConfigService
	agentFinderService      AgentFinderService
	st                      Backend
	cloudService            CloudService
	storageAccess           StorageInterface
	pool                    Pool
	authorizer              Authorizer
	check                   *common.BlockChecker
	resources               facade.Resources
	leadership              Leadership
	store                   objectstore.ObjectStore
	controllerStore         objectstore.ObjectStore

	keyUpdaterService  KeyUpdaterService
	machineService     MachineService
	networkService     NetworkService
	modelConfigService ModelConfigService

	logger corelogger.Logger
}

// NewMachineManagerAPI creates a new server-side MachineManager API facade.
func NewMachineManagerAPI(
	model coremodel.ModelInfo,
	controllerConfigService ControllerConfigService,
	agentFinderService AgentFinderService,
	backend Backend,
	cloudService CloudService,
	machineService MachineService,
	store, controllerStore objectstore.ObjectStore,
	storageAccess StorageInterface,
	pool Pool,
	auth Authorizer,
	resources facade.Resources,
	leadership Leadership,
	logger corelogger.Logger,
	networkService NetworkService,
	keyUpdaterService KeyUpdaterService,
	modelConfigService ModelConfigService,
	blockCommandService BlockCommandService,
) *MachineManagerAPI {
	api := &MachineManagerAPI{
		model:                   model,
		controllerConfigService: controllerConfigService,
		agentFinderService:      agentFinderService,
		st:                      backend,
		cloudService:            cloudService,
		machineService:          machineService,
		store:                   store,
		controllerStore:         controllerStore,
		pool:                    pool,
		authorizer:              auth,
		check:                   common.NewBlockChecker(blockCommandService),
		resources:               resources,
		leadership:              leadership,
		storageAccess:           storageAccess,
		logger:                  logger,
		networkService:          networkService,
		keyUpdaterService:       keyUpdaterService,
		modelConfigService:      modelConfigService,
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
		if p.Placement.Scope != mm.model.UUID.String() {
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

	jobs, err := common.StateJobs(p.Jobs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	template := state.MachineTemplate{
		Base:                    state.Base{OS: base.OS, Channel: base.Channel.String()},
		Constraints:             p.Constraints,
		Volumes:                 volumes,
		InstanceId:              p.InstanceId,
		Jobs:                    jobs,
		Nonce:                   p.Nonce,
		HardwareCharacteristics: p.HardwareCharacteristics,
		Addresses:               sAddrs,
		Placement:               placementDirective,
	}

	defer func() {
		if err == nil {
			// Ensure machine(s) exist in dqlite.
			err = mm.saveMachineInfo(ctx, result.Id())
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

func (mm *MachineManagerAPI) saveMachineInfo(ctx context.Context, machineName string) error {
	// This is temporary - just insert the machine id all al the parent ones.
	var errs []error
	for machineName != "" {
		_, err := mm.machineService.CreateMachine(ctx, coremachine.Name(machineName))
		// The machine might already exist e.g. if we are adding a subordinate
		// unit to an already existing machine. In this case, just continue
		// without error.
		if err != nil && !errors.Is(err, machineerrors.MachineAlreadyExists) {
			errs = append(errs, errors.Annotatef(err, "saving info for machine %q", machineName))
		}
		parent := names.NewMachineTag(machineName).Parent()
		if parent == nil {
			break
		}
		machineName = parent.Id()
	}
	if len(errs) == 0 {
		return nil
	}
	var errStr string
	for _, e := range errs {
		errStr += e.Error() + "\n"
	}
	return errors.New(errStr)
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
		AgentFinderService:      mm.agentFinderService,
		ObjectStore:             mm.controllerStore,
		KeyUpdaterService:       mm.keyUpdaterService,
		ModelConfigService:      mm.modelConfigService,
		MachineService:          mm.machineService,
	}

	icfg, err := InstanceConfig(
		ctx,
		mm.model.UUID,
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
			mm.logger.Errorf(context.TODO(),
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
		if err := mm.maybeUpdateInstanceStatus(p.All, m, map[string]interface{}{"transient": true}); err != nil {
			result.Results = append(result.Results, params.ErrorResult{Error: apiservererrors.ServerError(err)})
		}
	}
	return result, nil
}

func (mm *MachineManagerAPI) maybeUpdateInstanceStatus(all bool, m Machine, data map[string]interface{}) error {
	existingStatusInfo, err := m.InstanceStatus()
	if err != nil {
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
			return fmt.Errorf("machine %s is not in an error state (%v)", m.Id(), existingStatusInfo.Status)
		}
		// Otherwise just skip it.
		return nil
	}
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  existingStatusInfo.Status,
		Message: existingStatusInfo.Message,
		Data:    newData,
		Since:   &now,
	}
	return m.SetInstanceStatus(sInfo)
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

		if keep {
			mm.logger.Infof(context.TODO(), "destroy machine %v but keep instance", machineTag.Id())
			if err := mm.machineService.SetKeepInstance(ctx, coremachine.Name(machineTag.Id()), keep); err != nil {
				if !force {
					fail(err)
					continue
				}
				mm.logger.Warningf(context.TODO(), "could not keep instance for machine %v: %v", machineTag.Id(), err)
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

		units, err := machine.Units()
		if err != nil {
			fail(err)
			continue
		}
		for _, unit := range units {
			info.DestroyedUnits = append(info.DestroyedUnits, params.Entity{Tag: unit.UnitTag().String()})
		}

		info.DestroyedStorage, info.DetachedStorage, err = mm.classifyDetachedStorage(units)
		if err != nil {
			if !force {
				fail(err)
				continue
			}
			mm.logger.Warningf(context.TODO(), "could not deal with units' storage on machine %v: %v", machineTag.Id(), err)
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
			mm.logger.Warningf(context.TODO(), "could not unpin application leaders for machine %s with error %v", machineTag.Id(), err)
		}
		for _, result := range unpinResults.Results {
			if result.Error != nil {
				mm.logger.Warningf(context.TODO(),
					"could not unpin application leaders for machine %s with error %v", machineTag.Id(), result.Error)
			}
		}
		result.Info = &info
		results[i] = result
	}
	return params.DestroyMachineResults{Results: results}, nil
}

func (mm *MachineManagerAPI) destroyContainer(ctx context.Context, containers []string, force, keep, dryRun bool, maxWait time.Duration) ([]params.DestroyMachineResult, error) {
	if containers == nil || len(containers) == 0 {
		return nil, nil
	}
	entities := params.Entities{Entities: make([]params.Entity, len(containers))}
	for i, container := range containers {
		entities.Entities[i] = params.Entity{Tag: names.NewMachineTag(container).String()}
	}
	results, err := mm.destroyMachine(ctx, entities, force, keep, dryRun, maxWait)
	return results.Results, err
}

func (mm *MachineManagerAPI) classifyDetachedStorage(units []Unit) (destroyed, detached []params.Entity, _ error) {
	var storageErrors []params.ErrorResult
	storageError := func(e error) {
		storageErrors = append(storageErrors, params.ErrorResult{Error: apiservererrors.ServerError(e)})
	}

	storageSeen := names.NewSet()
	for _, unit := range units {
		storage, err := storagecommon.UnitStorage(mm.storageAccess, unit.UnitTag())
		if err != nil {
			storageError(errors.Annotatef(err, "getting storage for unit %v", unit.UnitTag().Id()))
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
			storageError(errors.Annotatef(err, "classifying storage for destruction for unit %v", unit.UnitTag().Id()))
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
