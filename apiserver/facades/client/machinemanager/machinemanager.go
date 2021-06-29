// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/environs/config"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.machinemanager")

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
	storageAccess    storageInterface
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

	clientLogger := logger.Child("client")
	options := []charmhub.Option{
		// TODO (stickupkid): Get the http transport from the facade context
		charmhub.WithHTTPTransport(charmhub.DefaultHTTPTransport),
	}

	var chCfg charmhub.Config
	chURL, ok := modelCfg.CharmHubURL()
	if ok {
		chCfg, err = charmhub.CharmHubConfigFromURL(chURL, clientLogger, options...)
	} else {
		chCfg, err = charmhub.CharmHubConfig(clientLogger, options...)
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
	*MachineManagerAPI
}

// NewFacadeV4 creates a new server-side MachineManager API facade.
func NewFacadeV4(ctx facade.Context) (*MachineManagerAPIV4, error) {
	machineManagerAPIV5, err := NewFacadeV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineManagerAPIV4{machineManagerAPIV5}, nil
}

// NewFacadeV5 creates a new server-side MachineManager API facade.
func NewFacadeV5(ctx facade.Context) (*MachineManagerAPIV5, error) {
	machineManagerAPIv6, err := NewFacadeV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineManagerAPIV5{machineManagerAPIv6}, nil
}

// NewFacadeV6 creates a new server-side MachineManager API facade.
func NewFacadeV6(ctx facade.Context) (*MachineManagerAPIV6, error) {
	machineManagerAPI, err := NewFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineManagerAPIV6{machineManagerAPI}, nil
}

// NewMachineManagerAPI creates a new server-side MachineManager API facade.
func NewMachineManagerAPI(
	backend Backend,
	storageAccess storageInterface,
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

	if p.Series == "" {
		model, err := mm.st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		conf, err := model.Config()
		if err != nil {
			return nil, errors.Trace(err)
		}
		p.Series = config.PreferredSeries(conf)
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

// DestroyMachine removes a set of machines from the model.
func (mm *MachineManagerAPI) DestroyMachine(args params.Entities) (params.DestroyMachineResults, error) {
	return mm.destroyMachine(args, false, false, time.Duration(0))
}

// ForceDestroyMachine forcibly removes a set of machines from the model.
// TODO (anastasiamac 2019-4-24) From Juju 3.0 this call will be removed in favour of DestroyMachinesWithParams.
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
	destroyMachine := func(entity params.Entity) params.DestroyMachineResult {
		result := params.DestroyMachineResult{}
		fail := func(e error) params.DestroyMachineResult {
			result.Error = apiservererrors.ServerError(e)
			return result
		}

		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			return fail(err)
		}
		machine, err := mm.st.Machine(machineTag.Id())
		if err != nil {
			return fail(err)
		}
		if keep {
			logger.Infof("destroy machine %v but keep instance", machineTag.Id())
			if err := machine.SetKeepInstance(keep); err != nil {
				if !force {
					return fail(err)
				}
				logger.Warningf("could not keep instance for machine %v: %v", machineTag.Id(), err)
			}
		}
		var info params.DestroyMachineInfo
		units, err := machine.Units()
		if err != nil {
			return fail(err)
		}

		var storageErrors []params.ErrorResult
		storageError := func(e error) {
			storageErrors = append(storageErrors, params.ErrorResult{Error: apiservererrors.ServerError(e)})
		}

		storageSeen := names.NewSet()
		for _, unit := range units {
			info.DestroyedUnits = append(
				info.DestroyedUnits,
				params.Entity{Tag: unit.UnitTag().String()},
			)
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

			destroyed, detached, err := storagecommon.ClassifyDetachedStorage(
				mm.storageAccess.VolumeAccess(), mm.storageAccess.FilesystemAccess(), storage)
			if err != nil {
				storageError(errors.Annotatef(err, "classifying storage for destruction for unit %v", unit.UnitTag().Id()))
				continue
			}
			info.DestroyedStorage = append(info.DestroyedStorage, destroyed...)
			info.DetachedStorage = append(info.DetachedStorage, detached...)
		}

		if len(storageErrors) != 0 {
			all := params.ErrorResults{Results: storageErrors}
			if !force {
				return fail(all.Combine())
			}
			logger.Warningf("could not deal with units' storage on machine %v: %v", machineTag.Id(), all.Combine())
		}

		applicationNames, err := mm.leadership.GetMachineApplicationNames(machineTag.Id())
		if err != nil {
			return fail(err)
		}

		if force {
			if err := machine.ForceDestroy(maxWait); err != nil {
				return fail(err)
			}
		} else {
			if err := machine.Destroy(); err != nil {
				return fail(err)
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
		results, err := mm.leadership.UnpinApplicationLeadersByName(machineTag, applicationNames)
		if err != nil {
			logger.Warningf("could not unpin application leaders for machine %s with error %v", machineTag.Id(), err)
		}
		for _, result := range results.Results {
			if result.Error != nil {
				logger.Warningf(
					"could not unpin application leaders for machine %s with error %v", machineTag.Id(), result.Error)
			}
		}

		result.Info = &info
		return result
	}
	results := make([]params.DestroyMachineResult, len(args.Entities))
	for i, entity := range args.Entities {
		results[i] = destroyMachine(entity)
	}
	return params.DestroyMachineResults{Results: results}, nil
}

// UpgradeSeriesValidate validates that the incoming arguments correspond to a
// valid series upgrade for the target machine.
// If they do, a list of the machine's current units is returned for use in
// soliciting user confirmation of the command.
func (mm *MachineManagerAPI) UpgradeSeriesValidate(
	args params.UpdateSeriesArgs,
) (params.UpgradeSeriesUnitsResults, error) {
	entities := make([]ValidationEntity, len(args.Args))
	for i, arg := range args.Args {
		entities[i] = ValidationEntity{
			Tag:    arg.Entity.Tag,
			Series: arg.Series,
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

// UpgradeSeriesPrepare prepares a machine for a OS series upgrade.
func (mm *MachineManagerAPI) UpgradeSeriesPrepare(arg params.UpdateSeriesArg) (params.ErrorResult, error) {
	if err := mm.authorizer.CanWrite(); err != nil {
		return params.ErrorResult{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResult{}, err
	}
	err := mm.upgradeSeriesAPI.Prepare(arg.Entity.Tag, arg.Series, arg.Force)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

// UpgradeSeriesComplete marks a machine as having completed a managed series
// upgrade.
func (mm *MachineManagerAPI) UpgradeSeriesComplete(arg params.UpdateSeriesArg) (params.ErrorResult, error) {
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
func (mm *MachineManagerAPIV4) UpdateMachineSeries(_ params.UpdateSeriesArgs) (params.ErrorResults, error) {
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
