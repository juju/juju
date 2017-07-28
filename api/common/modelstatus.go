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

	results := make([]base.ModelStatus, len(result.Results))
	for i, r := range result.Results {
		model, err := names.ParseModelTag(r.ModelTag)
		if err != nil {
			return nil, errors.Annotatef(err, "ModelTag %q at position %d", r.ModelTag, i)
		}
		owner, err := names.ParseUserTag(r.OwnerTag)
		if err != nil {
			return nil, errors.Annotatef(err, "OwnerTag %q at position %d", r.OwnerTag, i)
		}

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

		results[i] = base.ModelStatus{
			UUID:               model.Id(),
			Life:               string(r.Life),
			Owner:              owner.Id(),
			HostedMachineCount: r.HostedMachineCount,
			ServiceCount:       r.ApplicationCount,
			TotalMachineCount:  len(r.Machines),
			Volumes:            volumes,
			Filesystems:        filesystems,
		}
		results[i].Machines = make([]base.Machine, len(r.Machines))
		for j, mm := range r.Machines {
			if mm.Hardware != nil && mm.Hardware.Cores != nil {
				results[i].CoreCount += int(*mm.Hardware.Cores)
			}
			results[i].Machines[j] = base.Machine{
				Id:         mm.Id,
				InstanceId: mm.InstanceId,
				HasVote:    mm.HasVote,
				WantsVote:  mm.WantsVote,
				Status:     mm.Status,
			}
		}
	}
	return results, nil
}
