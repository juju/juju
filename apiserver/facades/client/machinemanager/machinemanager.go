// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os"
	"github.com/juju/os/series"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.machinemanager")

// MachineManagerAPI provides access to the MachineManager API facade.
type MachineManagerAPI struct {
	st            Backend
	storageAccess storageInterface
	pool          Pool
	authorizer    facade.Authorizer
	check         *common.BlockChecker
	resources     facade.Resources

	modelTag    names.ModelTag
	callContext context.ProviderCallContext
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
	return NewMachineManagerAPI(backend, storageAccess, pool, ctx.Auth(), model.ModelTag(), state.CallContext(st), ctx.Resources())
}

// Version 4 of MachineManagerAPI
type MachineManagerAPIV4 struct {
	*MachineManagerAPIV5
}

// Version 5 of Machine Manger API. Adds CreateUpgradeSeriesLock.
type MachineManagerAPIV5 struct {
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
	machineManagerAPI, err := NewFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineManagerAPIV5{machineManagerAPI}, nil
}

// NewMachineManagerAPI creates a new server-side MachineManager API facade.
func NewMachineManagerAPI(
	backend Backend,
	storageAccess storageInterface,
	pool Pool,
	auth facade.Authorizer,
	modelTag names.ModelTag,
	callCtx context.ProviderCallContext,
	resources facade.Resources,
) (*MachineManagerAPI, error) {
	if !auth.AuthClient() {
		return nil, common.ErrPerm
	}
	return &MachineManagerAPI{
		st:            backend,
		storageAccess: storageAccess,
		pool:          pool,
		authorizer:    auth,
		check:         common.NewBlockChecker(backend),
		modelTag:      modelTag,
		callContext:   callCtx,
		resources:     resources,
	}, nil
}

func (mm *MachineManagerAPI) checkCanWrite() error {
	return mm.checkAccess(permission.WriteAccess)
}

func (mm *MachineManagerAPI) checkCanRead() error {
	return mm.checkAccess(permission.ReadAccess)
}

func (mm *MachineManagerAPI) checkAccess(access permission.Access) error {
	canAccess, err := mm.authorizer.HasPermission(access, mm.modelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canAccess {
		return common.ErrPerm
	}
	return nil
}

// AddMachines adds new machines with the supplied parameters.
func (mm *MachineManagerAPI) AddMachines(args params.AddMachines) (params.AddMachinesResults, error) {
	results := params.AddMachinesResults{
		Machines: make([]params.AddMachinesResult, len(args.MachineParams)),
	}
	if err := mm.checkCanWrite(); err != nil {
		return results, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, p := range args.MachineParams {
		m, err := mm.addOneMachine(p)
		results.Machines[i].Error = common.ServerError(err)
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
				volumeParams, volumeAttachmentParams,
			})
		}
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
		Addresses:               params.NetworkAddresses(p.Addrs...),
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
	return mm.destroyMachine(args, false, false)
}

// ForceDestroyMachine forcibly removes a set of machines from the model.
func (mm *MachineManagerAPI) ForceDestroyMachine(args params.Entities) (params.DestroyMachineResults, error) {
	return mm.destroyMachine(args, true, false)
}

// DestroyMachineWithParams removes a set of machines from the model.
func (mm *MachineManagerAPI) DestroyMachineWithParams(args params.DestroyMachinesParams) (params.DestroyMachineResults, error) {
	entities := params.Entities{Entities: make([]params.Entity, len(args.MachineTags))}
	for i, tag := range args.MachineTags {
		entities.Entities[i].Tag = tag
	}
	return mm.destroyMachine(entities, args.Force, args.Keep)
}

func (mm *MachineManagerAPI) destroyMachine(args params.Entities, force, keep bool) (params.DestroyMachineResults, error) {
	if err := mm.checkCanWrite(); err != nil {
		return params.DestroyMachineResults{}, err
	}
	if err := mm.check.RemoveAllowed(); err != nil {
		return params.DestroyMachineResults{}, err
	}
	destroyMachine := func(entity params.Entity) (*params.DestroyMachineInfo, error) {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			return nil, err
		}
		machine, err := mm.st.Machine(machineTag.Id())
		if err != nil {
			return nil, err
		}
		if keep {
			logger.Infof("destroy machine %v but keep instance", machineTag.Id())
			if err := machine.SetKeepInstance(keep); err != nil {
				return nil, err
			}
		}
		var info params.DestroyMachineInfo
		units, err := machine.Units()
		if err != nil {
			return nil, err
		}
		storageSeen := names.NewSet()
		for _, unit := range units {
			info.DestroyedUnits = append(
				info.DestroyedUnits,
				params.Entity{Tag: unit.UnitTag().String()},
			)
			storage, err := storagecommon.UnitStorage(mm.storageAccess, unit.UnitTag())
			if err != nil {
				return nil, err
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
				return nil, err
			}
			info.DestroyedStorage = append(info.DestroyedStorage, destroyed...)
			info.DetachedStorage = append(info.DetachedStorage, detached...)
		}
		destroy := machine.Destroy
		if force {
			destroy = machine.ForceDestroy
		}
		if err := destroy(); err != nil {
			return nil, err
		}
		return &info, nil
	}
	results := make([]params.DestroyMachineResult, len(args.Entities))
	for i, entity := range args.Entities {
		info, err := destroyMachine(entity)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.DestroyMachineResults{results}, nil
}

// UpgradeSeriesValidate validates that the incoming arguments correspond to a
// valid series upgrade for the target machine.
// If they do, a list of the machine's current units is returned for use in
// soliciting user confirmation of the command.
func (mm *MachineManagerAPI) UpgradeSeriesValidate(
	args params.UpdateSeriesArgs,
) (params.UpgradeSeriesUnitsResults, error) {
	err := mm.checkCanRead()
	if err != nil {
		return params.UpgradeSeriesUnitsResults{}, err
	}

	results := make([]params.UpgradeSeriesUnitsResult, len(args.Args))
	for i, arg := range args.Args {
		machine, err := mm.machineFromTag(arg.Entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}

		err = mm.validateSeries(arg.Series, machine.Series(), arg.Entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}

		unitNames, err := mm.verifiedUnits(machine, arg.Series, arg.Force)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].UnitNames = unitNames
	}

	return params.UpgradeSeriesUnitsResults{Results: results}, nil
}

// UpgradeSeriesPrepare prepares a machine for a OS series upgrade.
func (mm *MachineManagerAPI) UpgradeSeriesPrepare(args params.UpdateSeriesArg) (params.ErrorResult, error) {
	if err := mm.checkCanWrite(); err != nil {
		return params.ErrorResult{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResult{}, err
	}
	err := mm.upgradeSeriesPrepare(args)
	if err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

func (mm *MachineManagerAPI) upgradeSeriesPrepare(arg params.UpdateSeriesArg) error {
	if arg.Series == "" {
		return &params.Error{
			Message: "series missing from args",
			Code:    params.CodeBadRequest,
		}
	}
	machineTag, err := names.ParseMachineTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	machine, err := mm.st.Machine(machineTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	unitNames, err := mm.verifiedUnits(machine, arg.Series, arg.Force)
	if err != nil {
		return errors.Trace(err)
	}

	if err = machine.CreateUpgradeSeriesLock(unitNames, arg.Series); err != nil {
		// TODO 2018-06-28 managed series upgrade
		// improve error handling based on error type, there will be cases where retrying
		// the hooks is needed etc.
		return errors.Trace(err)
	}
	defer func() {
		if err != nil {
			if err2 := machine.RemoveUpgradeSeriesLock(); err2 != nil {
				err = errors.Annotatef(err, "%s occurred while cleaning up from", err2)
			}
		}
	}()
	return nil
}

// UpgradeSeriesComplete marks a machine as having completed a managed series upgrade.
func (mm *MachineManagerAPI) UpgradeSeriesComplete(args params.UpdateSeriesArg) (params.ErrorResult, error) {
	if err := mm.checkCanWrite(); err != nil {
		return params.ErrorResult{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResult{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResult{}, err
	}
	err := mm.completeUpgradeSeries(args)
	if err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}

	return params.ErrorResult{}, nil
}

func (mm *MachineManagerAPI) completeUpgradeSeries(arg params.UpdateSeriesArg) error {
	machine, err := mm.machineFromTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	return machine.CompleteUpgradeSeries()
}

func (mm *MachineManagerAPI) removeUpgradeSeriesLock(arg params.UpdateSeriesArg) error {
	machine, err := mm.machineFromTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	return machine.RemoveUpgradeSeriesLock()
}

// WatchUpgradeSeriesNotifications returns a watcher that fires on upgrade series events.
func (mm *MachineManagerAPI) WatchUpgradeSeriesNotifications(args params.Entities) (params.NotifyWatchResults, error) {
	err := mm.checkCanRead()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		watcherId := ""
		machine, err := mm.st.Machine(tag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		w, err := machine.WatchUpgradeSeriesNotifications()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		watcherId = mm.resources.Register(w)
		result.Results[i].NotifyWatcherId = watcherId
	}
	return result, nil
}

// GetUpgradeSeriesMessages returns all new messages associated with upgrade
// series events. Messages that have already been retrieved once are not
// returned by this method.
func (mm *MachineManagerAPI) GetUpgradeSeriesMessages(args params.UpgradeSeriesNotificationParams) (params.StringsResults, error) {
	if err := mm.checkCanRead(); err != nil {
		return params.StringsResults{}, err
	}
	results := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Params)),
	}
	for i, param := range args.Params {
		machine, err := mm.machineFromTag(param.Entity.Tag)
		if err != nil {
			err = errors.Trace(err)
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		messages, finished, err := machine.GetUpgradeSeriesMessages()
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if finished {
			// If there are no more messages we stop the watcher resource.
			err = mm.resources.Stop(param.WatcherId)
			if err != nil {
				results.Results[i].Error = common.ServerError(err)
				continue
			}
		}
		results.Results[i].Result = messages
	}
	return results, nil
}

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

// verifiedUnits verifies that the machine units and their tree of subordinates
// all support the input series. If not, an error is returned.
// If they do, the agent statuses are checked to ensure that they are all in
// the idle state i.e. not installing, running hooks, or needing intervention.
// the final check is that the unit itself is not in an error state.
func (mm *MachineManagerAPI) verifiedUnits(machine Machine, series string, force bool) ([]string, error) {
	principals := machine.Principals()
	units, err := machine.VerifyUnitsSeries(principals, series, force)
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitNames := make([]string, len(units))
	for i, u := range units {
		agentStatus, err := u.AgentStatus()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if agentStatus.Status != status.Idle {
			return nil, errors.Errorf("unit %s is not ready to start a series upgrade; its agent status is: %q %s",
				u.Name(), agentStatus.Status, agentStatus.Message)
		}
		unitStatus, err := u.Status()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if unitStatus.Status == status.Error {
			return nil, errors.Errorf("unit %s is not ready to start a series upgrade; its status is: \"error\" %s",
				u.Name(), unitStatus.Message)
		}

		unitNames[i] = u.UnitTag().Id()
	}
	return unitNames, nil
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
	return version2 > version1, nil
}

// DEPRECATED: UpdateMachineSeries updates the series of the given machine(s) as well as all
// units and subordinates installed on the machine(s).
func (mm *MachineManagerAPI) UpdateMachineSeries(args params.UpdateSeriesArgs) (params.ErrorResults, error) {
	if err := mm.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := mm.updateOneMachineSeries(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (mm *MachineManagerAPI) updateOneMachineSeries(arg params.UpdateSeriesArg) error {
	if arg.Series == "" {
		return &params.Error{
			Message: "series missing from args",
			Code:    params.CodeBadRequest,
		}
	}
	machine, err := mm.machineFromTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	if arg.Series == machine.Series() {
		return nil // no-op
	}
	return machine.UpdateMachineSeries(arg.Series, arg.Force)
}

func (mm *MachineManagerAPI) validateSeries(argumentSeries, currentSeries string, machineTag string) error {
	if argumentSeries == "" {
		return &params.Error{
			Message: "series missing from args",
			Code:    params.CodeBadRequest,
		}
	}

	opSys, err := series.GetOSFromSeries(argumentSeries)
	if err != nil {
		return errors.Trace(err)
	}
	if opSys != os.Ubuntu {
		return errors.Errorf("series %q is from OS %q and is not a valid upgrade target",
			argumentSeries, opSys.String())
	}

	opSys, err = series.GetOSFromSeries(currentSeries)
	if err != nil {
		return errors.Trace(err)
	}
	if opSys != os.Ubuntu {
		return errors.Errorf("%s is running %s and is not valid for Ubuntu series upgrade",
			machineTag, opSys.String())
	}

	if argumentSeries == currentSeries {
		return errors.Errorf("%s is already running series %s", machineTag, argumentSeries)
	}

	isOlderSeries, err := isSeriesLessThan(argumentSeries, currentSeries)
	if err != nil {
		return errors.Trace(err)
	}
	if isOlderSeries {
		return errors.Errorf("machine %s is running %s which is a newer series than %s.",
			machineTag, currentSeries, argumentSeries)
	}

	return nil
}

// Applications returns for each input machine, the unique list of application
// names represented by the units running on the machine.
func (mm *MachineManagerAPI) Applications(args params.Entities) (params.StringsResults, error) {
	err := mm.checkCanRead()
	if err != nil {
		return params.StringsResults{}, err
	}

	results := make([]params.StringsResult, len(args.Entities))
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := mm.st.Machine(tag.Id())
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		units, err := machine.Units()
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}

		apps := make(map[string]bool)
		for _, unit := range units {
			apps[unit.ApplicationName()] = true
		}
		for app := range apps {
			results[i].Result = append(results[i].Result, app)
		}
	}

	return params.StringsResults{Results: results}, nil
}
