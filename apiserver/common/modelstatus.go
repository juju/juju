// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
)

// ModelStatusAPI implements the ModelStatus() API.
type ModelStatusAPI struct {
	authorizer facade.Authorizer
	apiUser    names.UserTag
	backend    ModelManagerBackend
}

// NewModelStatusAPI creates an implementation providing the ModelStatus() API.
func NewModelStatusAPI(st ModelManagerBackend, authorizer facade.Authorizer, apiUser names.UserTag) *ModelStatusAPI {
	return &ModelStatusAPI{
		authorizer: authorizer,
		apiUser:    apiUser,
		backend:    st,
	}
}

func (s *ModelStatusAPI) checkHasAdmin() error {
	isAdmin, err := s.authorizer.HasPermission(permission.SuperuserAccess, s.backend.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !isAdmin {
		return ServerError(ErrPerm)
	}
	return nil
}

// modelAuthCheck checks if the user is acting on their own behalf, or if they
// are an administrator acting on behalf of another user.
func (s *ModelStatusAPI) modelAuthCheck(modelTag names.ModelTag, owner names.UserTag) error {
	if err := s.checkHasAdmin(); err == nil {
		logger.Tracef("%q is a controller admin", s.apiUser.Id())
		return nil
	}
	if s.apiUser == owner {
		return nil
	}
	isAdmin, err := s.authorizer.HasPermission(permission.AdminAccess, modelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if isAdmin {
		return nil
	}
	return ErrPerm
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
		if st, err = c.backend.ForModel(modelTag); err != nil {
			return status, errors.Trace(err)
		}
		defer st.Close()
	}

	model, err := st.Model()
	if err != nil {
		return status, errors.Trace(err)
	}
	if err := c.modelAuthCheck(modelTag, model.Owner()); err != nil {
		return status, errors.Trace(err)
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
