// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
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
	return NewMachineManagerAPI(backend, storageAccess, pool, ctx.Auth(), model.ModelTag(), state.CallContext(st))
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
	}, nil
}

func (mm *MachineManagerAPI) checkCanWrite() error {
	canWrite, err := mm.authorizer.HasPermission(permission.WriteAccess, mm.modelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite {
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
		Series:      p.Series,
		Constraints: p.Constraints,
		Volumes:     volumes,
		InstanceId:  p.InstanceId,
		Jobs:        jobs,
		Nonce:       p.Nonce,
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
				params.Entity{unit.UnitTag().String()},
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

// UpgradeSeriesPrepare prepares a machine for a OS series upgrade.
func (mm *MachineManagerAPI) UpgradeSeriesPrepare(args params.UpdateSeriesArg) (params.ErrorResult, error) {
	if err := mm.checkCanWrite(); err != nil {
		return params.ErrorResult{}, err
	}
	if err := mm.check.ChangeAllowed(); err != nil {
		return params.ErrorResult{}, err
	}
	err := mm.updateSeriesPrepare(args)
	if err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

func (mm *MachineManagerAPI) updateSeriesPrepare(arg params.UpdateSeriesArg) error {
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
	err = mm.validateSeries(arg.Series, machine.Series(), machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	principals := machine.Principals()
	units, err := machine.VerifyUnitsSeries(principals, arg.Series, arg.Force)
	if err != nil {
		return errors.Trace(err)
	}
	unitNames := make([]string, len(units))
	for i := range units {
		unitNames[i] = units[i].UnitTag().Id()
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

	// TODO (hml) 2018-06-26 managed series upgrade
	// Next steps in updateSeriesPrepare:
	// 1. verify success of pre-upgrade-series hook run on each unit
	// 2. stop each unit's agent on machine
	// 3. write systemd unit agent files if necessary and copy tools based on series.

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
	err := mm.completeUpgradeSeries(args)
	if err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}

	return params.ErrorResult{}, nil
}

// DEPRECATED: UpdateMachineSeries updates the series of the given machine(s) as well as all
// units and subordintes installed on the machine(s).
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
	machineTag, err := names.ParseMachineTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	machine, err := mm.st.Machine(machineTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if arg.Series == machine.Series() {
		return nil // no-op
	}
	return machine.UpdateMachineSeries(arg.Series, arg.Force)
}

func (mm *MachineManagerAPI) completeUpgradeSeries(arg params.UpdateSeriesArg) error {
	machineTag, err := names.ParseMachineTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	machine, err := mm.st.Machine(machineTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	return machine.CompleteUpgradeSeries()
}

// [TODO](externalreality) We still need this, eventually the lock is going to cleaned up
// RemoveUpgradeSeriesLock removes a series upgrade prepare lock for a
// given machine.
func (mm *MachineManagerAPI) removeUpgradeSeriesLock(arg params.UpdateSeriesArg) error {
	machineTag, err := names.ParseMachineTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	machine, err := mm.st.Machine(machineTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	return machine.RemoveUpgradeSeriesLock()
}

func (mm *MachineManagerAPI) validateSeries(argumentSeries, currentSeries string, machineTag names.MachineTag) error {
	if argumentSeries == currentSeries {
		return errors.Errorf("%s is already running series %s", machineTag, argumentSeries)
	}
	isOlderSeries, err := isSeriesLessThan(argumentSeries, currentSeries)
	if err != nil {
		return err
	}
	if isOlderSeries {
		return errors.Errorf("machine %s is running %s which is a newer series than %s.", machineTag, currentSeries, argumentSeries)
	}

	return nil
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
