// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"context"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/resources"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// applicationIDGetter is a function type used to retrieve a coreapplication.ID
// based on the given context (from application name or unit name)
// It returns an error if the ID retrieval fails.
type applicationIDGetter func(ctx context.Context) (coreapplication.UUID, error)

// NewUnitFacade returns the resources portion of the uniter's API facade.
func NewUnitFacade(
	appOrUnitTag names.Tag,
	applicationService ApplicationService,
	resourceService ResourceService,
) (*UnitFacade, error) {
	var applicationIDGetter applicationIDGetter
	switch tag := appOrUnitTag.(type) {
	case names.UnitTag:
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			return nil, errors.Capture(err)
		}
		applicationIDGetter = func(ctx context.Context) (coreapplication.UUID, error) {
			return applicationService.GetApplicationUUIDByUnitName(ctx, unitName)
		}
	case names.ApplicationTag:
		applicationIDGetter = func(ctx context.Context) (coreapplication.UUID, error) {
			return applicationService.GetApplicationUUIDByName(ctx, tag.Id())
		}
	default:
		return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
	}

	return &UnitFacade{
		resourceService:         resourceService,
		getApplicationIDFromAPI: applicationIDGetter,
	}, nil
}

// UnitFacade is the resources portion of the uniter's API facade.
type UnitFacade struct {
	resourceService         ResourceService
	getApplicationIDFromAPI applicationIDGetter
	applicationID           coreapplication.UUID
}

// getApplicationID retrieves and caches the application ID for the unit.
// It fetches from the API if not already cached.
func (uf *UnitFacade) getApplicationID(ctx context.Context) (coreapplication.UUID, error) {
	if uf.applicationID == "" {
		applicationID, err := uf.getApplicationIDFromAPI(ctx)
		if err != nil {
			return uf.applicationID, err
		}
		uf.applicationID = applicationID
	}
	return uf.applicationID, nil
}

// listResources retrieves the application resources information through the
// resource service using the application ID.
func (uf *UnitFacade) listResources(ctx context.Context) ([]resource.Resource, error) {
	appID, err := uf.getApplicationID(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get application id: %w", err)
	}
	return uf.resourceService.GetResourcesByApplicationID(ctx, appID)
}

// GetResourceInfo returns the resource info for each of the given
// resource names (for the implicit application). If any one is missing then
// the corresponding result is set with errors.NotFound.
func (uf *UnitFacade) GetResourceInfo(ctx context.Context, args params.ListUnitResourcesArgs) (params.
	UnitResourcesResult, error) {
	var r params.UnitResourcesResult
	r.Resources = make([]params.UnitResourceResult, len(args.ResourceNames))

	// Avoid to fetch resources if not required
	if len(args.ResourceNames) == 0 {
		return r, nil
	}

	foundResources, err := uf.listResources(ctx)
	if err != nil {
		r.Error = apiservererrors.ServerError(err)
		return r, nil
	}

	for i, name := range args.ResourceNames {
		res, ok := lookUpResource(name, foundResources)
		if !ok {
			r.Resources[i].Error = apiservererrors.ServerError(jujuerrors.NotFoundf("resource %q", name))
			continue
		}

		r.Resources[i].Resource = resources.Resource2API(res)
	}
	return r, nil
}

// lookUpResource searches for a resource by name in a list of resources and
// returns the resource and a bool indicating success.
func lookUpResource(name string, resources []resource.Resource) (resource.Resource, bool) {
	for _, res := range resources {
		if name == res.Name {
			return res, true
		}
	}
	return resource.Resource{}, false
}
