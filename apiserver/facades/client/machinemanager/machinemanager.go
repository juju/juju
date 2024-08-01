// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs/config"
	environscontext "github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

var ClassifyDetachedStorage = storagecommon.ClassifyDetachedStorage

// ControllerConfigService defines a method for getting the controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
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

// MachineService manages machines.
type MachineService interface {
	CreateMachine(context.Context, machine.Name) (string, error)
	DeleteMachine(context.Context, machine.Name) error
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

// MachineManagerAPI provides access to the MachineManager API facade.
type MachineManagerAPI struct {
	controllerConfigService ControllerConfigService
	st                      Backend
	cloudService            common.CloudService
	credentialService       common.CredentialService
	storageAccess           StorageInterface
	pool                    Pool
	authorizer              Authorizer
	check                   *common.BlockChecker
	resources               facade.Resources
	leadership              Leadership
	store                   objectstore.ObjectStore
	controllerStore         objectstore.ObjectStore

	machineService MachineService
	networkService NetworkService

	credentialInvalidatorGetter environscontext.ModelCredentialInvalidatorGetter
	logger                      corelogger.Logger
}

// NewFacadeV11 create a new server-side MachineManager API facade. This
// is used for facade registration.
func NewFacadeV11(ctx facade.ModelContext) (*MachineManagerAPI, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := ctx.ServiceFactory()

	prechecker, err := stateenvirons.NewInstancePrechecker(st, serviceFactory.Cloud(), serviceFactory.Credential())
	if err != nil {
		return nil, errors.Trace(err)
	}

	backend := &stateShim{
		State:     st,
		prechcker: prechecker,
	}
	storageAccess, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pool := &poolShim{ctx.StatePool()}

	var leadership Leadership
	leadership, err = common.NewLeadershipPinningFromContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger := ctx.Logger().Child("machinemanager")

	controllerConfigService := serviceFactory.ControllerConfig()

	return NewMachineManagerAPI(
		controllerConfigService,
		backend,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Machine(),
		ctx.ObjectStore(),
		ctx.ControllerObjectStore(),
		storageAccess,
		pool,
		ModelAuthorizer{
			ModelTag:   model.ModelTag(),
			Authorizer: ctx.Auth(),
		},
		credentialcommon.CredentialInvalidatorGetter(ctx),
		ctx.Resources(),
		leadership,
		logger,
		ctx.ServiceFactory().Network(),
	)
}

// NewMachineManagerAPI creates a new server-side MachineManager API facade.
func NewMachineManagerAPI(
	controllerConfigService ControllerConfigService,
	backend Backend,
	cloudService common.CloudService,
	credentialService common.CredentialService,
	machineService MachineService,
	store, controllerStore objectstore.ObjectStore,
	storageAccess StorageInterface,
	pool Pool,
	auth Authorizer,
	credentialInvalidatorGetter environscontext.ModelCredentialInvalidatorGetter,
	resources facade.Resources,
	leadership Leadership,
	logger corelogger.Logger,
	networkService NetworkService,
) (*MachineManagerAPI, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	api := &MachineManagerAPI{
		controllerConfigService:     controllerConfigService,
		st:                          backend,
		cloudService:                cloudService,
		credentialService:           credentialService,
		machineService:              machineService,
		store:                       store,
		controllerStore:             controllerStore,
		pool:                        pool,
		authorizer:                  auth,
		check:                       common.NewBlockChecker(backend),
		credentialInvalidatorGetter: credentialInvalidatorGetter,
		resources:                   resources,
		leadership:                  leadership,
		storageAccess:               storageAccess,
		logger:                      logger,
		networkService:              networkService,
	}
	return api, nil
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
		model, err := mm.st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		conf, err := model.Config()
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
		model, err := mm.st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// For 1.21 we should support both UUID and name, and with 1.22
		// just support UUID
		if p.Placement.Scope != model.Name() && p.Placement.Scope != model.UUID() {
			return nil, fmt.Errorf("invalid model name %q", p.Placement.Scope)
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
		_, err := mm.machineService.CreateMachine(ctx, machine.Name(machineName))
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
		CredentialService:       mm.credentialService,
		ControllerConfigService: mm.controllerConfigService,
		ObjectStore:             mm.controllerStore,
	}

	icfg, err := InstanceConfig(ctx, st, mm.st, services, args.MachineId, args.Nonce, args.DataDir)
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
	model, err := mm.st.Model()
	if err != nil {
		return result, apiservererrors.ServerError(errors.Annotate(
			err, "getting model config",
		))
	}
	if args.DisablePackageCommands {
		icfg.EnableOSRefreshUpdate = false
		icfg.EnableOSUpgrade = false
	} else if cfg, err := model.Config(); err != nil {
		return result, apiservererrors.ServerError(errors.Annotate(
			err, "getting model config",
		))
	} else {
		icfg.EnableOSUpgrade = cfg.EnableOSUpgrade()
		icfg.EnableOSRefreshUpdate = cfg.EnableOSRefreshUpdate()
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
		machine, err := mm.st.Machine(machineTag.Id())
		if err != nil {
			fail(err)
			continue
		}
		if keep {
			mm.logger.Infof("destroy machine %v but keep instance", machineTag.Id())
			if err := machine.SetKeepInstance(keep); err != nil {
				if !force {
					fail(err)
					continue
				}
				mm.logger.Warningf("could not keep instance for machine %v: %v", machineTag.Id(), err)
			}
		}
		info := params.DestroyMachineInfo{
			MachineId: machineTag.Id(),
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
			mm.logger.Warningf("could not deal with units' storage on machine %v: %v", machineTag.Id(), err)
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
			mm.logger.Warningf("could not unpin application leaders for machine %s with error %v", machineTag.Id(), err)
		}
		for _, result := range unpinResults.Results {
			if result.Error != nil {
				mm.logger.Warningf(
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
