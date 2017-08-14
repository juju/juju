// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.machinemanager")

// MachineManagerAPI provides access to the MachineManager API facade.
type MachineManagerAPI struct {
	st         stateInterface
	authorizer facade.Authorizer
	check      *common.BlockChecker
}

type MachineManagerAPIV4 struct {
	*MachineManagerAPI
}

var getState = func(st *state.State) stateInterface {
	return stateShim{st}
}

// NewMachineManagerAPIV4 creates a new server-side MachineManager API facade.
func NewMachineManagerAPIV4(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*MachineManagerAPIV4, error) {

	machineManagerAPI, err := NewMachineManagerAPI(st, resources, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineManagerAPIV4{machineManagerAPI}, nil
}

// NewMachineManagerAPI creates a new server-side MachineManager API facade.
func NewMachineManagerAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*MachineManagerAPI, error) {

	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	s := getState(st)
	return &MachineManagerAPI{
		st:         s,
		authorizer: authorizer,
		check:      common.NewBlockChecker(s),
	}, nil
}

func (mm *MachineManagerAPI) checkCanWrite() error {
	canWrite, err := mm.authorizer.HasPermission(permission.WriteAccess, mm.st.ModelTag())
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
		conf, err := mm.st.ModelConfig()
		if err != nil {
			return nil, errors.Trace(err)
		}
		p.Series = config.PreferredSeries(conf)
	}

	var placementDirective string
	if p.Placement != nil {
		env, err := mm.st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// For 1.21 we should support both UUID and name, and with 1.22
		// just support UUID
		if p.Placement.Scope != env.Name() && p.Placement.Scope != env.UUID() {
			return nil, fmt.Errorf("invalid model name %q", p.Placement.Scope)
		}
		placementDirective = p.Placement.Directive
	}

	volumes := make([]state.MachineVolumeParams, 0, len(p.Disks))
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
			volumes = append(volumes, state.MachineVolumeParams{
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
func (mm *MachineManagerAPIV4) DestroyMachineWithParams(args params.DestroyMachinesParams) (params.DestroyMachineResults, error) {
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
		storageSeen := make(set.Tags)
		for _, unit := range units {
			info.DestroyedUnits = append(
				info.DestroyedUnits,
				params.Entity{unit.UnitTag().String()},
			)
			storage, err := common.UnitStorage(mm.st, unit.UnitTag())
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

			destroyed, detached := common.ClassifyDetachedStorage(storage)
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
