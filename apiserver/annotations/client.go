// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service contains api calls for accessing service functionality.
package annotations

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.annotations")

func init() {
	common.RegisterStandardFacade("Annotations", 0, NewAPI)
}

// Annotations defines the methods on the service API end point.
type Annotations interface {
	Get(args params.Entities) params.AnnotationsGetResults
	Set(args params.AnnotationsSet) params.ErrorResults
}

// API implements the service interface and is the concrete
// implementation of the api end point.
type API struct {
	state      *state.State
	authorizer common.Authorizer
}

var (
	_ state.GlobalEntity = (*state.Machine)(nil)
	_ state.GlobalEntity = (*state.Unit)(nil)
	_ state.GlobalEntity = (*state.Service)(nil)
	_ state.GlobalEntity = (*state.Charm)(nil)
	_ state.GlobalEntity = (*state.Environment)(nil)
)

// NewAPI returns a new charm annotator API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		state:      st,
		authorizer: authorizer,
	}, nil
}

// Get returns annotations for given entities.
// If annotations cannot be retrieved for a given entity, an error is returned.
// Each entity is treated independently and, hence, will fail or succeed independently.
func (api *API) Get(args params.Entities) params.AnnotationsGetResults {
	entityResults := []params.AnnotationsGetResult{}
	for _, entity := range args.Entities {
		anEntityResult := params.AnnotationsGetResult{Entity: entity}
		if annts, err := api.getEntityAnnotations(entity.Tag); err != nil {
			logger.Warningf("Could not get annotations for entity [%v] because %v", entity.Tag, err)
			anEntityResult.Error = params.ErrorResult{annotateError(err, entity.Tag)}
		} else {
			logger.Infof("Annotations for entity [%v] are %v", entity.Tag, annts)
			anEntityResult.Annotations = annts
		}
		entityResults = append(entityResults, anEntityResult)
	}
	return params.AnnotationsGetResults{Results: entityResults}
}

// Set stores annotations for given entities
func (api *API) Set(args params.AnnotationsSet) params.ErrorResults {
	allErrors := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Collection.Entities)),
	}
	for i, entity := range args.Collection.Entities {
		if err := api.setEntityAnnotations(entity.Tag, args.Annotations); err != nil {
			logger.Warningf("Could not set annotations for entity [%v] because %v", entity.Tag, err)
			allErrors.Results[i].Error = annotateError(err, entity.Tag)
		}
	}
	return allErrors
}

func annotateError(err error, tag string) *params.Error {
	return common.ServerError(
		errors.Trace(
			errors.Annotatef(
				err, "while setting annotations to %q", tag)))
}

func (api *API) getEntityAnnotations(entityTag string) (map[string]string, error) {
	tag, err := names.ParseTag(entityTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	entity, err := api.findEntity(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	annotations, err := state.Annotations(entity, api.state)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return annotations, nil
}

func (api *API) findEntity(tag names.Tag) (state.GlobalEntity, error) {
	entity0, err := api.state.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	entity, ok := entity0.(state.GlobalEntity)
	if !ok {
		return nil, common.NotSupportedError(tag, "annotations")
	}
	return entity, nil
}

func (api *API) setEntityAnnotations(entityTag string, annotations map[string]string) error {
	tag, err := names.ParseTag(entityTag)
	if err != nil {
		return errors.Trace(err)
	}
	entity, err := api.findEntity(tag)
	if err != nil {
		return errors.Trace(err)
	}
	return state.SetAnnotations(entity, api.state, annotations)
}
