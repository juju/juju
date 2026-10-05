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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainresource "github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// applicationUUIDGetter is a function type used to retrieve a
// coreapplication.UUID based on the given context (from application name or
// unit name)
// It returns an error if the UUID retrieval fails.
type applicationUUIDGetter func(ctx context.Context) (coreapplication.UUID, error)

// NewUnitFacade returns the resources portion of the uniter's API facade.
func NewUnitFacade(
	appOrUnitTag names.Tag,
	applicationService ApplicationService,
	resourceService ResourceService,
) (*UnitFacade, error) {
	var applicationUUIDGetter applicationUUIDGetter
	var applicationName string
	var unitName coreunit.Name
	switch tag := appOrUnitTag.(type) {
	case names.UnitTag:
		var err error
		unitName, err = coreunit.NewName(tag.Id())
		if err != nil {
			return nil, errors.Capture(err)
		}
		applicationUUIDGetter = func(ctx context.Context) (coreapplication.UUID, error) {
			return applicationService.GetApplicationUUIDByUnitName(ctx, unitName)
		}
		applicationName = unitName.Application()
	case names.ApplicationTag:
		applicationName = tag.Id()
		applicationUUIDGetter = func(ctx context.Context) (coreapplication.UUID, error) {
			return applicationService.GetApplicationUUIDByName(ctx, applicationName)
		}
	default:
		return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
	}

	return &UnitFacade{
		resourceService:           resourceService,
		applicationService:        applicationService,
		getApplicationUUIDFromAPI: applicationUUIDGetter,
		applicationName:           applicationName,
		unitName:                  unitName,
	}, nil
}

// UnitFacade is the resources portion of the uniter's API facade.
type UnitFacade struct {
	resourceService           ResourceService
	applicationService        ApplicationService
	getApplicationUUIDFromAPI applicationUUIDGetter
	applicationID             coreapplication.UUID
	applicationName           string
	unitName                  coreunit.Name
}

// getApplicationUUID retrieves and caches the application UUID for the unit.
// It fetches from the API if not already cached.
func (uf *UnitFacade) getApplicationUUID(ctx context.Context) (coreapplication.UUID, error) {
	if uf.applicationID == "" {
		applicationID, err := uf.getApplicationUUIDFromAPI(ctx)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return "", jujuerrors.NotFoundf("unit %q", uf.unitName)
		} else if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return "", jujuerrors.NotFoundf("application %q", uf.applicationName)
		} else if err != nil {
			return uf.applicationID, err
		}
		uf.applicationID = applicationID
	}
	return uf.applicationID, nil
}

// listResources retrieves the application resources information through the
// resource service using the application UUID.
func (uf *UnitFacade) listResources(ctx context.Context) ([]resource.Resource, error) {
	appID, err := uf.getApplicationUUID(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get application UUID: %w", err)
	}
	return uf.resourceService.GetResourcesByApplicationUUID(ctx, appID)
}

// GetResourceInfo returns the resource info for each of the given
// resource names (for the implicit application). If any one is missing then
// the corresponding result is set with errors.NotFound.
func (uf *UnitFacade) GetResourceInfo(ctx context.Context, args params.ListUnitResourcesArgs) (params.UnitResourcesResult, error) {
	var r params.UnitResourcesResult
	r.Resources = make([]params.UnitResourceResult, len(args.ResourceNames))

	// Avoid to fetch resources if not required
	if len(args.ResourceNames) == 0 {
		return r, nil
	}

	foundResources, err := uf.listResources(ctx)
	if err != nil {
		switch {
		case errors.Is(err, applicationerrors.UnitNotFound):
			err = jujuerrors.NotFoundf("unit %q", uf.unitName)
		case errors.Is(err, applicationerrors.ApplicationNotFound):
			err = jujuerrors.NotFoundf("application %q", uf.applicationName)
		default:
			err = errors.Errorf("cannot list resources: %w", err)
		}
		r.Error = apiservererrors.ServerError(err)
		return r, nil
	}

	for i, name := range args.ResourceNames {
		res, ok := lookUpResource(name, foundResources)
		if ok {
			r.Resources[i].Resource = resources.Resource2API(res)
			continue
		}
		if uf.unitName == "" {
			r.Resources[i].Error = apiservererrors.ServerError(jujuerrors.NotFoundf("resource %q", name))
			continue
		}

		res, err = uf.getUnitResource(ctx, name)
		switch {
		case errors.Is(err, applicationerrors.UnitNotFound):
			r.Error = apiservererrors.ServerError(
				jujuerrors.NotFoundf("unit %q", uf.unitName),
			)
		case errors.Is(err, applicationerrors.ApplicationNotFound):
			r.Error = apiservererrors.ServerError(
				jujuerrors.NotFoundf("application %q", uf.applicationName),
			)
		case errors.Is(err, resourceerrors.ResourceNotFound):
			r.Resources[i].Error = apiservererrors.ServerError(jujuerrors.NotFoundf("resource %q", name))
		default:
			r.Resources[i].Error = apiservererrors.ServerError(err)
		}

		r.Resources[i].Resource = resources.Resource2API(res)
	}
	return r, nil
}

func (uf *UnitFacade) getUnitResource(ctx context.Context, name string) (resource.Resource, error) {
	resourceUUID, err := uf.resourceService.GetUnitResourceID(ctx, domainresource.GetUnitResourceIDArgs{
		UnitName: uf.unitName,
		Name:     name,
	})
	if err != nil {
		return resource.Resource{}, errors.Capture(err)
	}
	return uf.resourceService.GetResourceWithoutApplication(ctx, resourceUUID)
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
