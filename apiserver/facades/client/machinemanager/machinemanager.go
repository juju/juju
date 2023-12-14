// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/core/instance"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/environs/manual/winrmprovisioner"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.machinemanager")

var ClassifyDetachedStorage = storagecommon.ClassifyDetachedStorage

// Leadership represents a type for modifying the leadership settings of an
// application for series upgrades.
type Leadership interface {
	// GetMachineApplicationNames returns the applications associated with a
	// machine.
	GetMachineApplicationNames(string) ([]string, error)

	// UnpinApplicationLeadersByName takes a slice of application names and
	// attempts to unpin them accordingly.
	UnpinApplicationLeadersByName(names.Tag, []string) (params.PinApplicationsResults, error)
}

// Authorizer checks to see if an operation can be performed.
type Authorizer interface {
	// CanRead checks to see if a read is possible. Returns an error if a read
	// is not possible.
	CanRead() error

	// CanWrite checks to see if a write is possible. Returns an error if a
	// write is not possible.
	CanWrite() error

	// AuthClient returns true if the entity is an external user.
	AuthClient() bool
}

// CharmhubClient represents a way for querying the charmhub api for information
// about the application charm.
type CharmhubClient interface {
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// MachineManagerAPI provides access to the MachineManager API facade.
type MachineManagerAPI struct {
	st               Backend
	storageAccess    StorageInterface
	pool             Pool
	authorizer       Authorizer
	check            *common.BlockChecker
	resources        facade.Resources
	leadership       Leadership
	upgradeSeriesAPI UpgradeSeries

	callContext environscontext.ProviderCallContext
}

// NewFacade create a new server-side MachineManager API facade. This
// is used for facade registration.
func NewFacade(ctx facade.Context) (*MachineManagerAPI, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backend := &stateShim{State: st}
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

	modelCfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}

	options := []charmhub.Option{
		// TODO (stickupkid): Get the http transport from the facade context
		charmhub.WithHTTPTransport(charmhub.DefaultHTTPTransport),
	}

	var chCfg charmhub.Config
	chURL, ok := modelCfg.CharmHubURL()
	if ok {
		chCfg, err = charmhub.CharmHubConfigFromURL(chURL, logger, options...)
	} else {
		chCfg, err = charmhub.CharmHubConfig(logger, options...)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	chClient, err := charmhub.NewClient(chCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewMachineManagerAPI(
		backend,
		storageAccess,
		pool,
		ModelAuthorizer{
			ModelTag:   model.ModelTag(),
			Authorizer: ctx.Auth(),
		},
		environscontext.CallContext(st),
		ctx.Resources(),
		leadership,
		chClient,
	)
}

// MachineManagerAPIV4 defines the Version 4 of MachineManagerAPI
type MachineManagerAPIV4 struct {
	*MachineManagerAPIV5
}

// MachineManagerAPIV5 defines the Version 5 of Machine Manager API.
// Adds CreateUpgradeSeriesLock and removes UpdateMachineSeries.
type MachineManagerAPIV5 struct {
	*MachineManagerAPIV6
}

// MachineManagerAPIV6 defines the Version 6 of Machine Manager API.
// Changes input parameters to DestroyMachineWithParams and ForceDestroyMachine.
type MachineManagerAPIV6 struct {
	*MachineManagerAPIV7
}

// MachineManagerAPIV7 defines the Version 7 of Machine Manager API.
type MachineManagerAPIV7 struct {
	*MachineManagerAPIV8
}

// MachineManagerAPIV8 defines the Version 8 of Machine Manager API.
type MachineManagerAPIV8 struct {
	*MachineManagerAPIV9
}

// MachineManagerAPIV9 defines the Version 9 of Machine Manager API.
type MachineManagerAPIV9 struct {
	*MachineManagerAPI
}

// NewMachineManagerAPI creates a new server-side MachineManager API facade.
func NewMachineManagerAPI(
	backend Backend,
	storageAccess StorageInterface,
	pool Pool,
	auth Authorizer,
	callCtx environscontext.ProviderCallContext,
	resources facade.Resources,
	leadership Leadership,
	charmhubClient CharmhubClient,
) (*MachineManagerAPI, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	api := &MachineManagerAPI{
		st:            backend,
		storageAccess: storageAccess,
		pool:          pool,
		authorizer:    auth,
		check:         common.NewBlockChecker(backend),
		callContext:   callCtx,
		resources:     resources,
		leadership:    leadership,
		upgradeSeriesAPI: NewUpgradeSeriesAPI(
			upgradeSeriesState{state: backend},
			makeUpgradeSeriesValidator(charmhubClient),
			auth,
		),
	}
	return api, nil
}

// AddMachines adds new machines with the supplied parameters.
// The args will contain machine series.
func (mm *MachineManagerAPIV7) AddMachines(args params.AddMachines) (params.AddMachinesResults, error) {
	for i, arg := range args.MachineParams {
		if arg.Series == "" {
			continue
		}
		base, err := series.GetBaseFromSeries(arg.Series)
		if err != nil {
			continue
		}
		arg.Base = &params.Base{
			Name:    base.Name,
			Channel: base.Channel.String(),
		}
		args.MachineParams[i] = arg
	}
	return mm.MachineManagerAPI.AddMachines(args)
}

// compatibilityMachineParams ensures that AddMachine called from a juju 3.x
// client will work against a juju 2.9.x controller. In juju 3.x,
// params.AddMachineParams was changed to remove series however, the facade
// version was not changed, nor was the name of the params.AddMachineParams
// changed. Thus it appears you can use a juju 3.x client to deploy from a
// juju 2.9 controller, which then fails because the series was not found.
// Make those corrections here.
func compatibilityMachineParams(arg params.AddMachineParams) (params.AddMachineParams, error) {
	if arg.Base == nil {
		return arg, nil
	}
	machineSeries, err := series.GetSeriesFromChannel(arg.Base.Name, arg.Base.Channel)
	if err != nil {
		return arg, err
	}
	arg.Series = machineSeries
	return arg, nil
}

// AddMachines adds new machines with the supplied parameters.
// The args will contain Base info.
func (mm *MachineManagerAPI) AddMachines(args params.AddMachines) (params.AddMachinesResults, error) {
	results := params.AddMachinesResults{
		Machines: make([]params.AddMachinesResult, len(args.MachineParams)),
	}
	if err := mm.authorizer.CanWrite(); err != nil {
		return results, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, p := range args.MachineParams {
		var err error
		p, err = compatibilityMachineParams(p)
		if err != nil {
			results.Machines[i].Error = apiservererrors.ServerError(errors.Annotatef(err, "compatibility updates of arg failed"))
			continue
		}
		m, err := mm.addOneMachine(p)
		results.Machines[i].Error = apiservererrors.ServerError(err)
		if err == nil {
			results.Machines[i].Machine = m.Id()
		}
	}
	return results, nil
}

func (mm *MachineManagerAPI) addOneMachine(p params.AddMachineParams) (*state.Machine, error) {
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

	// TODO(wallyworld) - from here on we still expect series.
	// Future work will convert downstream to use Base.
	if p.Series == "" {
		if p.Base == nil {
			model, err := mm.st.Model()
			if err != nil {
				return nil, errors.Trace(err)
			}
			conf, err := model.Config()
			if err != nil {
				return nil, errors.Trace(err)
			}
			p.Series = config.PreferredSeries(conf)
		} else {
			var err error
			p.Series, err = series.GetSeriesFromChannel(p.Base.Name, p.Base.Channel)
			if err != nil {
				return nil, errors.Trace(err)
			}
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
	sAddrs, err := params.ToProviderAddresses(p.Addrs...).ToSpaceAddresses(mm.st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	jobs, err := common.StateJobs(p.Jobs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	template := state.MachineTemplate{
		Series:                  p.Series,
		Constraints:             p.Constraints,
		Volumes:                 volumes,
		InstanceId:              p.InstanceId,
		Jobs:                    jobs,
		Nonce:                   p.Nonce,
		HardwareCharacteristics: p.HardwareCharacteristics,
		Addresses:               sAddrs,
		Placement:               placementDirective,
	}
	if p.ContainerType == "" {
		return mm.st.AddOneMachine(template)
	}
	if p.ParentId != "" {
		return mm.st.AddMachineInsideMachine(template, p.ParentId, p.ContainerType)
	}
	return mm.st.AddMachineInsideNewMachine(template, template, p.ContainerType)
}

// ProvisioningScript is not available via the V6 API.
func (api *MachineManagerAPIV6) ProvisioningScript(_ struct{}) {}

// ProvisioningScript returns a shell script that, when run,
// provisions a machine agent on the machine executing the script.
func (mm *MachineManagerAPI) ProvisioningScript(args params.ProvisioningScriptParams) (params.ProvisioningScriptResult, error) {
	if err := mm.authorizer.CanWrite(); err != nil {
		return params.ProvisioningScriptResult{}, err
	}

	var result params.ProvisioningScriptResult
	st, err := mm.pool.SystemState()
	if err != nil {
		return result, errors.Trace(err)
	}
	icfg, err := InstanceConfig(st, mm.st, args.MachineId, args.Nonce, args.DataDir)
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

	osSeries, err := series.GetOSFromSeries(icfg.Series)
	if err != nil {
		return result, apiservererrors.ServerError(errors.Annotatef(err,
			"cannot decide which provisioning script to generate based on this series %q", icfg.Series))
	}

	getProvisioningScript := sshprovisioner.ProvisioningScript
	if osSeries == coreos.Windows {
		getProvisioningScript = winrmprovisioner.ProvisioningScript
	}

	result.Script, err = getProvisioningScript(icfg)
	if err != nil {
		return result, apiservererrors.ServerError(errors.Annotate(
			err, "getting provisioning script",
		))
	}

	return result, nil
}

// RetryProvisioning is not available via the V6 API.
func (api *MachineManagerAPIV6) RetryProvisioning(_ struct{}) {}

// RetryProvisioning marks a provisioning error as transient on the machines.
func (mm *MachineManagerAPI) RetryProvisioning(p params.RetryProvisioningArgs) (params.ErrorResults, error) {
	if err := mm.authorizer.CanWrite(); err != nil {
		return params.ErrorResults{}, err
	}

	if err := mm.check.ChangeAllowed(); err != nil {
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

// DestroyMachine removes a set of machines from the model.
// TODO(juju3) - remove
func (mm *MachineManagerAPI) DestroyMachine(args params.Entities) (params.DestroyMachineResults, error) {
	return mm.destroyMachine(args, false, false, time.Duration(0))
}

// ForceDestroyMachine forcibly removes a set of machines from the model.
// TODO(juju3) - remove
// Also from ModelManger v6 this call is less useful as it does not support MaxWait customisation.
func (mm *MachineManagerAPI) ForceDestroyMachine(args params.Entities) (params.DestroyMachineResults, error) {
	return mm.destroyMachine(args, true, false, time.Duration(0))
}

// DestroyMachineWithParams removes a set of machines from the model.
// v5 and prior versions did not support MaxWait.
func (mm *MachineManagerAPIV5) DestroyMachineWithParams(args params.DestroyMachinesParams) (params.DestroyMachineResults, error) {
	entities := params.Entities{Entities: make([]params.Entity, len(args.MachineTags))}
	for i, tag := range args.MachineTags {
		entities.Entities[i].Tag = tag
	}
	return mm.destroyMachine(entities, args.Force, args.Keep, time.Duration(0))
}

// DestroyMachineWithParams removes a set of machines from the model.
func (mm *MachineManagerAPI) DestroyMachineWithParams(args params.DestroyMachinesParams) (params.DestroyMachineResults, error) {
	entities := params.Entities{Entities: make([]params.Entity, len(args.MachineTags))}
	for i, tag := range args.MachineTags {
		entities.Entities[i].Tag = tag
	}
	return mm.destroyMachine(entities, args.Force, args.Keep, common.MaxWait(args.MaxWait))
}

func (mm *MachineManagerAPI) destroyMachine(args params.Entities, force, keep bool, maxWait time.Duration) (params.DestroyMachineResults, error) {
	if err := mm.authorizer.CanWrite(); err != nil {
		return params.DestroyMachineResults{}, err
	}
	if err := mm.check.RemoveAllowed(); err != nil {
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
			logger.Infof("destroy machine %v but keep instance", machineTag.Id())
			if err := machine.SetKeepInstance(keep); err != nil {
				if !force {
					fail(err)
					continue
				}
				logger.Warningf("could not keep instance for machine %v: %v", machineTag.Id(), err)
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
		if force {
			info.DestroyedContainers, err = mm.destoryContainer(containers, force, keep, maxWait)
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
			logger.Warningf("could not deal with units' storage on machine %v: %v", machineTag.Id(), err)
		}

		applicationNames, err := mm.leadership.GetMachineApplicationNames(machineTag.Id())
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
			if err := machine.Destroy(); err != nil {
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
		unpinResults, err := mm.leadership.UnpinApplicationLeadersByName(machineTag, applicationNames)
		if err != nil {
			logger.Warningf("could not unpin application leaders for machine %s with error %v", machineTag.Id(), err)
		}
		for _, result := range unpinResults.Results {
			if result.Error != nil {
				logger.Warningf(
					"could not unpin application leaders for machine %s with error %v", machineTag.Id(), result.Error)
			}
		}

		result.Info = &info
		results[i] = result
	}
	return params.DestroyMachineResults{Results: results}, nil
}

func (mm *MachineManagerAPI) destoryContainer(containers []string, force, keep bool, maxWait time.Duration) ([]params.DestroyMachineResult, error) {
	if containers == nil || len(containers) == 0 {
		return nil, nil
	}
	entities := params.Entities{Entities: make([]params.Entity, len(containers))}
	for i, container := range containers {
		entities.Entities[i] = params.Entity{Tag: names.NewMachineTag(container).String()}
	}
	results, err := mm.destroyMachine(entities, force, keep, maxWait)
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

// UpgradeSeriesValidate validates that the incoming arguments correspond to a
// valid series upgrade for the target machine.
// If they do, a list of the machine's current units is returned for use in
// soliciting user confirmation of the command.
func (mm *MachineManagerAPI) UpgradeSeriesValidate(
	args params.UpdateChannelArgs,
) (params.UpgradeSeriesUnitsResults, error) {
	entities := make([]ValidationEntity, len(args.Args))
	for i, arg := range args.Args {
		argSeries, err := mm.seriesFromParams(arg)
		if err != nil {
			return params.UpgradeSeriesUnitsResults{}, apiservererrors.ServerError(err)
		}
		entities[i] = ValidationEntity{
			Tag:    arg.Entity.Tag,
			Series: argSeries,
			Force:  arg.Force,
		}
	}

	validations, err := mm.upgradeSeriesAPI.Validate(entities)
	if err != nil {
		return params.UpgradeSeriesUnitsResults{}, apiservererrors.ServerError(err)
	}

	results := params.UpgradeSeriesUnitsResults{
		Results: make([]params.UpgradeSeriesUnitsResult, len(validations)),
	}
	for i, v := range validations {
		if v.Error != nil {
			results.Results[i].Error = apiservererrors.ServerError(v.Error)
			continue
		}
		results.Results[i].UnitNames = v.UnitNames
	}
	return results, nil
}

func (mm *MachineManagerAPI) seriesFromParams(arg params.UpdateChannelArg) (string, error) {
	machineTag, err := names.ParseMachineTag(arg.Entity.Tag)
	if err != nil {
		return "", errors.Trace(err)
	}
	machine, err := mm.st.Machine(machineTag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	argSeries := arg.Series
	if argSeries == "" && arg.Channel != "" {
		base, err := series.GetBaseFromSeries(machine.Series())
		if err != nil {
			return "", errors.Trace(err)
		}
		argSeries, err = series.GetSeriesFromChannel(base.Name, arg.Channel)
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	return argSeries, nil
}

// UpgradeSeriesPrepare prepares a machine for a OS series upgrade.
func (mm *MachineManagerAPI) UpgradeSeriesPrepare(arg params.UpdateChannelArg) (params.ErrorResult, error) {
	if err := mm.authorizer.CanWrite(); err != nil {
		return params.ErrorResult{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResult{}, err
	}
	argSeries, err := mm.seriesFromParams(arg)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	err = mm.upgradeSeriesAPI.Prepare(arg.Entity.Tag, argSeries, arg.Force)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

// UpgradeSeriesComplete marks a machine as having completed a managed series
// upgrade.
func (mm *MachineManagerAPI) UpgradeSeriesComplete(arg params.UpdateChannelArg) (params.ErrorResult, error) {
	if err := mm.authorizer.CanWrite(); err != nil {
		return params.ErrorResult{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResult{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResult{}, err
	}
	err := mm.upgradeSeriesAPI.Complete(arg.Entity.Tag)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}

	return params.ErrorResult{}, nil
}

// WatchUpgradeSeriesNotifications returns a watcher that fires on upgrade
// series events.
func (mm *MachineManagerAPI) WatchUpgradeSeriesNotifications(args params.Entities) (params.NotifyWatchResults, error) {
	err := mm.authorizer.CanRead()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		var watcherID string
		machine, err := mm.st.Machine(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		w, err := machine.WatchUpgradeSeriesNotifications()
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watcherID = mm.resources.Register(w)
		result.Results[i].NotifyWatcherId = watcherID
	}
	return result, nil
}

// GetUpgradeSeriesMessages returns all new messages associated with upgrade
// series events. Messages that have already been retrieved once are not
// returned by this method.
func (mm *MachineManagerAPI) GetUpgradeSeriesMessages(args params.UpgradeSeriesNotificationParams) (params.StringsResults, error) {
	if err := mm.authorizer.CanRead(); err != nil {
		return params.StringsResults{}, err
	}
	results := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Params)),
	}
	for i, param := range args.Params {
		machine, err := mm.machineFromTag(param.Entity.Tag)
		if err != nil {
			err = errors.Trace(err)
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		messages, finished, err := machine.GetUpgradeSeriesMessages()
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if finished {
			// If there are no more messages we stop the watcher resource.
			err = mm.resources.Stop(param.WatcherId)
			if err != nil {
				results.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}
		results.Results[i].Result = messages
	}
	return results, nil
}

// TODO (stickupkid): This will eventually be removed once we extract all the
// other methods to commands.
func (mm *MachineManagerAPI) machineFromTag(tag string) (Machine, error) {
	machineTag, err := names.ParseMachineTag(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	machine, err := mm.st.Machine(machineTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return machine, nil
}

// isSeriesLessThan returns a bool indicating whether the first argument's
// version is lexicographically less than the second argument's, thus indicating
// that the series represents an older version of the operating system. The
// output is only valid for Ubuntu series.
func isSeriesLessThan(series1, series2 string) (bool, error) {
	version1, err := series.SeriesVersion(series1)
	if err != nil {
		return false, err
	}
	version2, err := series.SeriesVersion(series2)
	if err != nil {
		return false, err
	}
	// Versions may be numeric.
	vers1Int, err1 := strconv.Atoi(version1)
	vers2Int, err2 := strconv.Atoi(version2)
	if err1 == nil && err2 == nil {
		return vers2Int > vers1Int, nil
	}
	return version2 > version1, nil
}

// UpdateMachineSeries returns an error.
// DEPRECATED
func (mm *MachineManagerAPIV4) UpdateMachineSeries(_ params.UpdateChannelArgs) (params.ErrorResults, error) {
	return params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: apiservererrors.ServerError(errors.New("UpdateMachineSeries is no longer supported")),
		}},
	}, nil
}

// ModelAuthorizer defines if a given operation can be performed based on a
// model tag.
type ModelAuthorizer struct {
	ModelTag   names.ModelTag
	Authorizer facade.Authorizer
}

// CanRead checks to see if a read is possible. Returns an error if a read is
// not possible.
func (a ModelAuthorizer) CanRead() error {
	return a.checkAccess(permission.ReadAccess)
}

// CanWrite checks to see if a write is possible. Returns an error if a write
// is not possible.
func (a ModelAuthorizer) CanWrite() error {
	return a.checkAccess(permission.WriteAccess)
}

// AuthClient returns true if the entity is an external user.
func (a ModelAuthorizer) AuthClient() bool {
	return a.Authorizer.AuthClient()
}

func (a ModelAuthorizer) checkAccess(access permission.Access) error {
	canAccess, err := a.Authorizer.HasPermission(access, a.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canAccess {
		return apiservererrors.ErrPerm
	}
	return nil
}
