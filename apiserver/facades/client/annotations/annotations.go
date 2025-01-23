// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/annotation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// AnnotationService defines the methods on the annotation service API end
// point.
type AnnotationService interface {
	// GetCharmAnnotations returns the annotations for the given ID.
	GetAnnotations(context.Context, annotations.ID) (map[string]string, error)

	// GetCharmAnnotations returns the annotations for the given ID.
	GetCharmAnnotations(context.Context, annotation.GetCharmArgs) (map[string]string, error)

	// SetAnnotations sets the annotations for the given ID.
	SetAnnotations(context.Context, annotations.ID, map[string]string) error

	// SetCharmAnnotations sets the annotations for the given ID.
	SetCharmAnnotations(context.Context, annotation.GetCharmArgs, map[string]string) error
}

// API implements the service interface and is the concrete
// implementation of the api end point.
type API struct {
	modelTag          names.ModelTag
	authorizer        facade.Authorizer
	annotationService AnnotationService
}

func (api *API) checkCanRead(ctx context.Context) error {
	return api.authorizer.HasPermission(ctx, permission.ReadAccess, api.modelTag)
}

func (api *API) checkCanWrite(ctx context.Context) error {
	return api.authorizer.HasPermission(ctx, permission.WriteAccess, api.modelTag)
}

// Get returns annotations for given entities.
// If annotations cannot be retrieved for a given entity, an error is returned.
// Each entity is treated independently and, hence, will fail or succeed independently.
func (api *API) Get(ctx context.Context, args params.Entities) params.AnnotationsGetResults {
	if err := api.checkCanRead(ctx); err != nil {
		result := make([]params.AnnotationsGetResult, len(args.Entities))
		for i := range result {
			result[i].Error = params.ErrorResult{Error: apiservererrors.ServerError(err)}
		}
		return params.AnnotationsGetResults{Results: result}
	}

	entityResults := []params.AnnotationsGetResult{}
	for _, entity := range args.Entities {
		anEntityResult := params.AnnotationsGetResult{EntityTag: entity.Tag}
		if annts, err := api.getEntityAnnotations(ctx, entity.Tag); err != nil {
			anEntityResult.Error = params.ErrorResult{Error: annotateError(err, entity.Tag, "getting")}
		} else {
			anEntityResult.Annotations = annts
		}
		entityResults = append(entityResults, anEntityResult)
	}
	return params.AnnotationsGetResults{Results: entityResults}
}

// Set stores annotations for given entities
func (api *API) Set(ctx context.Context, args params.AnnotationsSet) params.ErrorResults {
	if err := api.checkCanWrite(ctx); err != nil {
		errorResults := make([]params.ErrorResult, len(args.Annotations))
		for i := range errorResults {
			errorResults[i].Error = apiservererrors.ServerError(err)
		}
		return params.ErrorResults{Results: errorResults}
	}
	setErrors := []params.ErrorResult{}
	for _, entityAnnotation := range args.Annotations {
		err := api.setEntityAnnotations(ctx, entityAnnotation.EntityTag, entityAnnotation.Annotations)
		if err != nil {
			setErrors = append(setErrors,
				params.ErrorResult{Error: annotateError(err, entityAnnotation.EntityTag, "setting")})
		}
	}
	return params.ErrorResults{Results: setErrors}
}

func annotateError(err error, tag, op string) *params.Error {
	return apiservererrors.ServerError(errors.Annotatef(err, "%v annotations for %q", op, tag))
}

func (api *API) getEntityAnnotations(ctx context.Context, entityTag string) (map[string]string, error) {
	tag, err := names.ParseTag(entityTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	switch tag.Kind() {
	case names.CharmTagKind:
		url, err := charm.ParseURL(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}

		results, err := api.annotationService.GetCharmAnnotations(ctx, annotation.GetCharmArgs{
			Source:   url.Schema,
			Name:     url.Name,
			Revision: url.Revision,
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return results, nil

	default:
		id, err := annotations.ConvertTagToID(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		results, err := api.annotationService.GetAnnotations(ctx, id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return results, nil
	}
}

func (api *API) setEntityAnnotations(ctx context.Context, entityTag string, values map[string]string) error {
	tag, err := names.ParseTag(entityTag)
	if err != nil {
		return errors.Trace(err)
	}

	switch tag.Kind() {
	case names.CharmTagKind:
		url, err := charm.ParseURL(tag.Id())
		if err != nil {
			return errors.Trace(err)
		}

		err = api.annotationService.SetCharmAnnotations(ctx, annotation.GetCharmArgs{
			Source:   url.Schema,
			Name:     url.Name,
			Revision: url.Revision,
		}, values)
		if err != nil {
			return errors.Trace(err)
		}
		return nil

	default:
		id, err := annotations.ConvertTagToID(tag)
		if err != nil {
			return errors.Trace(err)
		}
		return api.annotationService.SetAnnotations(ctx, id, values)
	}
}
