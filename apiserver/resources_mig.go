// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	jujuerrors "github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"

	"github.com/juju/juju/core/resource"
	domainresource "github.com/juju/juju/domain/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
)

type ResourceService interface {
	GetApplicationResourceID(ctx context.Context, args domainresource.GetApplicationResourceIDArgs) (resource.UUID, error)
	SetResource(ctx context.Context, args domainresource.SetResourceArgs) (domainresource.Resource, error)
	SetUnitResource(ctx context.Context, args domainresource.SetUnitResourceArgs) (domainresource.SetUnitResourceResult, error)
}

type ResourceServiceGetter interface {
	Resource(context.Context) (ResourceService, error)
}

type poolStateHelper interface {
	Release() bool
}

// resourcesMigrationUploadHandler handles resources uploads for model migrations.
type resourcesMigrationUploadHandler struct {
	ctxt                     httpContext
	stateAuthFunc            func(*http.Request) (poolStateHelper, error)
	resourceServiceGetter    ResourceServiceGetter
	applicationServiceGetter ApplicationServiceGetter
}

func (h *resourcesMigrationUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "POST":
		err = errors.Capture(h.ServePost(w, r))
	default:
		err = jujuerrors.MethodNotAllowedf("method not allowed: %s", r.Method)
	}
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", errors.Errorf("cannot return error to user: %w", err))
		}
	}
}

func (h *resourcesMigrationUploadHandler) ServePost(w http.ResponseWriter, r *http.Request) error {
	// Validate before authenticate because the authentication is dependent
	// on the state connection that is determined during the validation.
	st, err := h.stateAuthFunc(r)
	if err != nil {
		return jujuerrors.Unauthorizedf("auth failure: %w", err)
	}
	defer st.Release()

	resourceService, err := h.resourceServiceGetter.Resource(r.Context())
	if err != nil {
		return errors.Capture(err)
	}
	applicationService, err := h.applicationServiceGetter.Application(r.Context())
	if err != nil {
		return errors.Capture(err)
	}
	res, err := h.processPost(r, resourceService, applicationService)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(sendStatusAndJSON(w, http.StatusOK, &params.ResourceUploadResult{
		ID:        res.ID,
		Timestamp: res.Timestamp,
	}))
}

// processPost handles resources upload POST request after
// authentication.
func (h *resourcesMigrationUploadHandler) processPost(
	r *http.Request,
	resourceService ResourceService,
	applicationService ApplicationService,
) (resource.Resource, error) {
	var empty resource.Resource
	ctx := r.Context()
	query := r.URL.Query()

	userID := query.Get("user") // Is allowed to be blank

	// Get the target of the upload, which is an application with or without unit
	appID, unitUUID, err := getUploadTarget(ctx, applicationService, query)
	if err != nil {
		return empty, errors.Capture(err)
	}

	res, err := queryToResource(query)
	if err != nil {
		return empty, errors.Capture(err)
	}

	resUUID, err := resourceService.GetApplicationResourceID(ctx,
		domainresource.GetApplicationResourceIDArgs{
			ApplicationID: appID,
			Name:          res.Name,
		})
	if err != nil {
		return empty, errors.Capture(err)
	}

	result := resource.Resource{
		ID:            resUUID.String(),
		ApplicationID: appID.String(),
		Resource:      res,
		Username:      userID,
	}
	if err := result.Validate(); err != nil {
		return empty, errors.Errorf("bad resource metadata: %w", err)
	}

	if unitUUID != "" {
		res, err := resourceService.SetUnitResource(ctx,
			domainresource.SetUnitResourceArgs{
				ResourceUUID: resUUID,
				UnitUUID:     unitUUID,
			})
		if err != nil {
			return empty, errors.Capture(err)
		}
		result.Timestamp = res.Timestamp
	}

	if !isPlaceholder(query) {
		res, err := resourceService.SetResource(ctx, domainresource.SetResourceArgs{
			ApplicationID: appID,
			Resource:      res,
			Reader:        r.Body,
			Increment:     false,
		})
		if err != nil {
			return empty, errors.Capture(err)
		}
		result.Timestamp = res.Timestamp
		result.Resource = res.Resource
	}
	return result, nil
}

func isPlaceholder(query url.Values) bool {
	return query.Get("timestamp") == ""
}

func getUploadTarget(
	ctx context.Context,
	service ApplicationService,
	query url.Values,
) (appID coreapplication.ID, unitUUID coreunit.UUID, err error) {
	// Validate parameters
	appName := query.Get("application")
	unitName := query.Get("unit")
	switch {
	case appName == "" && unitName == "":
		return appID, unitUUID, jujuerrors.BadRequestf("missing application/unit")
	case appName != "" && unitName != "":
		return appID, unitUUID, jujuerrors.BadRequestf("application and unit can't be set at the same time")
	}

	var coreUnitName coreunit.Name
	// Resolve unitUUID if necessary
	if unitName != "" {
		coreUnitName, err = coreunit.NewName(unitName)
		if err != nil {
			return appID, unitUUID, jujuerrors.BadRequestf(err.Error())
		}
		unitUUID, err = service.GetUnitUUID(ctx, coreUnitName)
		if err != nil {
			return appID, unitUUID, errors.Capture(err)
		}
	}

	// resolve appUUID
	if appName == "" {
		appID, err = service.GetApplicationIDByUnitName(ctx, coreUnitName)
	} else {
		appID, err = service.GetApplicationIDByName(ctx, appName)
	}
	return appID, unitUUID, errors.Capture(err)
}

func queryToResource(query url.Values) (charmresource.Resource, error) {
	var err error
	empty := charmresource.Resource{}

	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        query.Get("name"),
			Path:        query.Get("path"),
			Description: query.Get("description"),
		},
	}
	if res.Name == "" {
		return empty, jujuerrors.BadRequestf("missing name")
	}
	res.Type, err = charmresource.ParseType(query.Get("type"))
	if err != nil {
		return empty, jujuerrors.BadRequestf("invalid type")
	}
	res.Origin, err = charmresource.ParseOrigin(query.Get("origin"))
	if err != nil {
		return empty, jujuerrors.BadRequestf("invalid origin")
	}
	res.Revision, err = strconv.Atoi(query.Get("revision"))
	if err != nil {
		return empty, jujuerrors.BadRequestf("invalid revision")
	}
	res.Size, err = strconv.ParseInt(query.Get("size"), 10, 64)
	if err != nil {
		return empty, jujuerrors.BadRequestf("invalid size")
	}
	switch res.Type {
	case charmresource.TypeFile:
		if res.Path == "" {
			return empty, jujuerrors.BadRequestf("missing path")
		}
		res.Fingerprint, err = charmresource.ParseFingerprint(query.Get("fingerprint"))
		if err != nil {
			return empty, jujuerrors.BadRequestf("invalid fingerprint")
		}
	case charmresource.TypeContainerImage:
		res.Fingerprint, err = charmresource.ParseFingerprint(query.Get("fingerprint"))
		if err != nil {
			// Old resources do not have fingerprints.
			res.Fingerprint = charmresource.Fingerprint{}
		}
	}
	return res, nil
}

type resourceServiceGetter struct {
	ctxt httpContext
}

func (rsg *resourceServiceGetter) Resource(ctx context.Context) (ResourceService, error) {
	domainServices, err := rsg.ctxt.domainServicesForRequest(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return domainServices.Resource(), nil
}
