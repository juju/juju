// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// ModelStatusAPI provides common client-side API functions
// to call into apiserver.common.ModelStatusAPI.
type ModelStatusAPI struct {
	facade base.FacadeCaller
}

// NewModelStatusAPI creates a ModelStatusAPI on the specified facade,
// and uses this name when calling through the caller.
func NewModelStatusAPI(facade base.FacadeCaller) *ModelStatusAPI {
	return &ModelStatusAPI{facade}
}

// ModelStatus returns a status summary for each model tag passed in.
func (c *ModelStatusAPI) ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error) {
	result := params.ModelStatusResults{}
	models := make([]params.Entity, len(tags))
	for i, tag := range tags {
		models[i] = params.Entity{Tag: tag.String()}
	}
	req := params.Entities{
		Entities: models,
	}
	if err := c.facade.FacadeCall("ModelStatus", req, &result); err != nil {
		return nil, err
	}

	return c.processModelStatusResults(result.Results)
}

func (c *ModelStatusAPI) processModelStatusResults(rs []params.ModelStatus) ([]base.ModelStatus, error) {
	results := make([]base.ModelStatus, len(rs))
	for i, r := range rs {
		if r.Error != nil {
			// cope with typed error
			results[i].Error = errors.Trace(r.Error)
			continue
		}
		model, err := names.ParseModelTag(r.ModelTag)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		owner, err := names.ParseUserTag(r.OwnerTag)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i] = constructModelStatus(model, owner, r)
	}
	return results, nil
}

func constructModelStatus(model names.ModelTag, owner names.UserTag, r params.ModelStatus) base.ModelStatus {
	volumes := make([]base.Volume, len(r.Volumes))
	for i, in := range r.Volumes {
		volumes[i] = base.Volume{
			Id:         in.Id,
			ProviderId: in.ProviderId,
			Status:     in.Status,
			Detachable: in.Detachable,
		}
	}

	filesystems := make([]base.Filesystem, len(r.Filesystems))
	for i, in := range r.Filesystems {
		filesystems[i] = base.Filesystem{
			Id:         in.Id,
			ProviderId: in.ProviderId,
			Status:     in.Status,
			Detachable: in.Detachable,
		}
	}

	result := base.ModelStatus{
		UUID:               model.Id(),
		Life:               string(r.Life),
		Owner:              owner.Id(),
		HostedMachineCount: r.HostedMachineCount,
		ApplicationCount:   r.ApplicationCount,
		TotalMachineCount:  len(r.Machines),
		Volumes:            volumes,
		Filesystems:        filesystems,
	}
	result.Machines = make([]base.Machine, len(r.Machines))
	for j, mm := range r.Machines {
		if mm.Hardware != nil && mm.Hardware.Cores != nil {
			result.CoreCount += int(*mm.Hardware.Cores)
		}
		result.Machines[j] = base.Machine{
			Id:         mm.Id,
			InstanceId: mm.InstanceId,
			HasVote:    mm.HasVote,
			WantsVote:  mm.WantsVote,
			Status:     mm.Status,
		}
	}
	return result
}
