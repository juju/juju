// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
)

// ModelStatusAPI implements the ModelStatus() API.
type ModelStatusAPI struct {
	authorizer facade.Authorizer
	apiUser    names.UserTag
	backend    ModelManagerBackend
	pool       BackendPool
}

// NewModelStatusAPI creates an implementation providing the ModelStatus() API.
func NewModelStatusAPI(st ModelManagerBackend, pool BackendPool, authorizer facade.Authorizer, apiUser names.UserTag) *ModelStatusAPI {
	return &ModelStatusAPI{
		authorizer: authorizer,
		apiUser:    apiUser,
		backend:    st,
		pool:       pool,
	}
}

// ModelStatus returns a summary of the model.
func (c *ModelStatusAPI) ModelStatus(req params.Entities) (params.ModelStatusResults, error) {
	models := req.Entities
	results := params.ModelStatusResults{}

	status := make([]params.ModelStatus, len(models))
	for i, model := range models {
		modelStatus, err := c.modelStatus(model.Tag)
		if err != nil {
			return results, errors.Trace(err)
		}
		status[i] = modelStatus
	}
	results.Results = status
	return results, nil
}

func (c *ModelStatusAPI) modelStatus(tag string) (params.ModelStatus, error) {
	var status params.ModelStatus
	modelTag, err := names.ParseModelTag(tag)
	if err != nil {
		return status, errors.Trace(err)
	}
	st := c.backend
	if modelTag != c.backend.ModelTag() {
		var releaser func()
		if st, releaser, err = c.pool.Get(modelTag.Id()); err != nil {
			return status, errors.Trace(err)
		}
		defer releaser()
	}

	model, err := st.Model()
	if err != nil {
		return status, errors.Trace(err)
	}
	isAdmin, err := HasModelAdmin(c.authorizer, c.apiUser, c.backend.ControllerTag(), model)
	if err != nil {
		return status, errors.Trace(err)
	}
	if !isAdmin {
		return status, ErrPerm
	}

	machines, err := st.AllMachines()
	if err != nil {
		return status, errors.Trace(err)
	}

	var hostedMachines []Machine
	for _, m := range machines {
		if !m.IsManager() {
			hostedMachines = append(hostedMachines, m)
		}
	}

	applications, err := st.AllApplications()
	if err != nil {
		return status, errors.Trace(err)
	}

	modelMachines, err := ModelMachineInfo(st)
	if err != nil {
		return status, errors.Trace(err)
	}

	return params.ModelStatus{
		ModelTag:           tag,
		OwnerTag:           model.Owner().String(),
		Life:               params.Life(model.Life().String()),
		HostedMachineCount: len(hostedMachines),
		ApplicationCount:   len(applications),
		Machines:           modelMachines,
	}, nil
}
