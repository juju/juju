// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// ModelStatusAPI provides common client-side API functions
// to call into apiserver.common.ModelStatusAPI.
type ModelStatusAPI struct {
	facade base.FacadeCaller
	legacy bool
}

// NewModelStatusAPI creates a ModelStatusAPI on the specified facade,
// and uses this name when calling through the caller.
func NewModelStatusAPI(facade base.FacadeCaller, legacy bool) *ModelStatusAPI {
	return &ModelStatusAPI{facade: facade, legacy: legacy}
}

// ModelStatus returns a status summary for each model tag passed in. If a
// given model is not found, the corresponding ModelStatus.Error field will
// contain an error matching errors.NotFound.
func (c *ModelStatusAPI) ModelStatus(ctx context.Context, tags ...names.ModelTag) ([]base.ModelStatus, error) {
	if c.legacy {
		return c.modelStatusCompat(ctx, tags...)
	}
	result := params.ModelStatusResults{}
	models := make([]params.Entity, len(tags))
	for i, tag := range tags {
		models[i] = params.Entity{Tag: tag.String()}
	}
	req := params.Entities{
		Entities: models,
	}
	if err := c.facade.FacadeCall(ctx, "ModelStatus", req, &result); err != nil {
		return nil, err
	}
	if len(result.Results) != len(tags) {
		return nil, errors.Errorf("%d results, expected %d", len(result.Results), len(tags))
	}
	return c.processModelStatusResults(result.Results)
}

func (c *ModelStatusAPI) processModelStatusResults(rs []params.ModelStatus) ([]base.ModelStatus, error) {
	results := make([]base.ModelStatus, len(rs))
	for i, r := range rs {
		if r.Error != nil {
			results[i].Error = params.TranslateWellKnownError(r.Error)
			continue
		}
		aModel, err := names.ParseModelTag(r.ModelTag)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i] = constructModelStatus(aModel, r)
	}
	return results, nil
}

func constructModelStatus(m names.ModelTag, r params.ModelStatus) base.ModelStatus {
	volumes := make([]base.Volume, len(r.Volumes))
	for i, in := range r.Volumes {
		volumes[i] = base.Volume{
			Id:         in.Id,
			ProviderId: in.ProviderId,
			Status:     in.Status,
			Message:    in.Message,
			Detachable: in.Detachable,
		}
	}

	filesystems := make([]base.Filesystem, len(r.Filesystems))
	for i, in := range r.Filesystems {
		filesystems[i] = base.Filesystem{
			Id:         in.Id,
			ProviderId: in.ProviderId,
			Status:     in.Status,
			Message:    in.Message,
			Detachable: in.Detachable,
		}
	}

	result := base.ModelStatus{
		UUID:               m.Id(),
		Life:               r.Life,
		Qualifier:          model.Qualifier(r.Qualifier),
		ModelType:          model.ModelType(r.Type),
		HostedMachineCount: r.HostedMachineCount,
		ApplicationCount:   r.ApplicationCount,
		UnitCount:          r.UnitCount,
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
			Id:          mm.Id,
			InstanceId:  mm.InstanceId,
			DisplayName: mm.DisplayName,
			Status:      mm.Status,
			Message:     mm.Message,
		}
	}
	result.Applications = transform.Slice(r.Applications, func(app params.ModelApplicationInfo) base.Application {
		return base.Application{Name: app.Name}
	})
	return result
}
