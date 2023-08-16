// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var getState = func(st *state.State, m *state.Model) annotationAccess {
	return stateShim{st, m}
}

// Annotations defines the methods on the service API end point.
type Annotations interface {
	Get(ctx context.Context, args params.Entities) params.AnnotationsGetResults
	Set(ctx context.Context, args params.AnnotationsSet) params.ErrorResults
}

// API implements the service interface and is the concrete
// implementation of the api end point.
type API struct {
	access     annotationAccess
	authorizer facade.Authorizer
}

func (api *API) checkCanRead() error {
	return api.authorizer.HasPermission(permission.ReadAccess, api.access.ModelTag())
}

func (api *API) checkCanWrite() error {
	return api.authorizer.HasPermission(permission.WriteAccess, api.access.ModelTag())
}

// Get returns annotations for given entities.
// If annotations cannot be retrieved for a given entity, an error is returned.
// Each entity is treated independently and, hence, will fail or succeed independently.
func (api *API) Get(ctx context.Context, args params.Entities) params.AnnotationsGetResults {
	if err := api.checkCanRead(); err != nil {
		result := make([]params.AnnotationsGetResult, len(args.Entities))
		for i := range result {
			result[i].Error = params.ErrorResult{Error: apiservererrors.ServerError(err)}
		}
		return params.AnnotationsGetResults{Results: result}
	}

	entityResults := []params.AnnotationsGetResult{}
	for _, entity := range args.Entities {
		anEntityResult := params.AnnotationsGetResult{EntityTag: entity.Tag}
		if annts, err := api.getEntityAnnotations(entity.Tag); err != nil {
			anEntityResult.Error = params.ErrorResult{annotateError(err, entity.Tag, "getting")}
		} else {
			anEntityResult.Annotations = annts
		}
		entityResults = append(entityResults, anEntityResult)
	}
	return params.AnnotationsGetResults{Results: entityResults}
}

// Set stores annotations for given entities
func (api *API) Set(ctx context.Context, args params.AnnotationsSet) params.ErrorResults {
	if err := api.checkCanWrite(); err != nil {
		errorResults := make([]params.ErrorResult, len(args.Annotations))
		for i := range errorResults {
			errorResults[i].Error = apiservererrors.ServerError(err)
		}
		return params.ErrorResults{Results: errorResults}
	}
	setErrors := []params.ErrorResult{}
	for _, entityAnnotation := range args.Annotations {
		err := api.setEntityAnnotations(entityAnnotation.EntityTag, entityAnnotation.Annotations)
		if err != nil {
			setErrors = append(setErrors,
				params.ErrorResult{Error: annotateError(err, entityAnnotation.EntityTag, "setting")})
		}
	}
	return params.ErrorResults{Results: setErrors}
}

func annotateError(err error, tag, op string) *params.Error {
	return apiservererrors.ServerError(
		errors.Trace(
			errors.Annotatef(
				err, "while %v annotations to %q", op, tag)))
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
	annotations, err := api.access.Annotations(entity)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return annotations, nil
}

func (api *API) findEntity(tag names.Tag) (state.GlobalEntity, error) {
	entity0, err := api.access.FindEntity(tag)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, apiservererrors.ErrPerm
		}
		return nil, err
	}
	entity, ok := entity0.(state.GlobalEntity)
	if !ok {
		return nil, apiservererrors.NotSupportedError(tag, "annotations")
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
	return api.access.SetAnnotations(entity, annotations)
}
