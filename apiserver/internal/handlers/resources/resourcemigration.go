// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"net/http"
	"net/url"

	"github.com/juju/errors"

	internalhttp "github.com/juju/juju/apiserver/internal/http"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// resourcesMigrationUploadHandler handles resources uploads for model migrations.
type resourcesMigrationUploadHandler struct {
	resourceServiceGetter    ResourceServiceGetter
	applicationServiceGetter ApplicationServiceGetter
	logger                   logger.Logger
}

// NewResourceMigrationUploadHandler returns a new HTTP handler for resources
// uploads during model migrations.
func NewResourceMigrationUploadHandler(
	applicationServiceGetter ApplicationServiceGetter,
	resourceServiceGetter ResourceServiceGetter,
	logger logger.Logger,
) *resourcesMigrationUploadHandler {
	return &resourcesMigrationUploadHandler{
		applicationServiceGetter: applicationServiceGetter,
		resourceServiceGetter:    resourceServiceGetter,
		logger:                   logger,
	}
}

// ServeHTTP handles HTTP requests by delegating to ServePost for POST requests
// or returning a method not allowed error for others.
func (h *resourcesMigrationUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "POST":
		err = internalerrors.Capture(h.servePost(w, r))
	default:
		err = errors.MethodNotAllowedf("method not allowed: %s", r.Method)
	}
	if err != nil {
		if err := internalhttp.SendError(w, internalerrors.Capture(err), h.logger); err != nil {
			h.logger.Errorf(context.TODO(), "cannot return error to user: %v", err)
		}
	}
}

// ServePost handles the POST request for resource uploads, including
// validation, authentication, processing, and response.
func (h *resourcesMigrationUploadHandler) servePost(w http.ResponseWriter, r *http.Request) error {
	// todo(gfouillet): This call should be authenticated. When model domain will
	//  provide authentication checks, we will need to ensure here that
	//  the request has been authenticated, and that the targeted model is in
	//  `importing` state.

	resourceService, err := h.resourceServiceGetter.Resource(r)
	if err != nil {
		return internalerrors.Capture(err)
	}

	applicationService, err := h.applicationServiceGetter.Application(r)
	if err != nil {
		return internalerrors.Capture(err)
	}

	res, err := h.processPost(r, resourceService, applicationService)
	if err != nil {
		return internalerrors.Capture(err)
	}
	return internalhttp.SendStatusAndJSON(w, http.StatusOK, &params.ResourceUploadResult{
		ID:        res.UUID.String(),
		Timestamp: res.Timestamp,
	})
}

// processPost handles resources upload POST request after
// authentication.
func (h *resourcesMigrationUploadHandler) processPost(
	r *http.Request,
	resourceService ResourceService,
	applicationService ApplicationService,
) (coreresource.Resource, error) {
	var empty coreresource.Resource
	ctx := r.Context()
	query := r.URL.Query()

	userID := query.Get("user") // Is allowed to be blank.

	// Get the target of the upload, which is an application with or without
	// unit.
	target, err := getUploadTarget(ctx, applicationService, query)
	if err != nil {
		return empty, internalerrors.Capture(err)
	}

	resUUID, err := resourceService.GetApplicationResourceID(ctx,
		resource.GetApplicationResourceIDArgs{
			ApplicationID: target.appID,
			Name:          target.name,
		})
	if err != nil {
		return empty, internalerrors.Errorf("resource upload failed: %w", err)
	}

	if target.unitUUID != "" {
		err := resourceService.SetUnitResource(ctx, resUUID, target.unitUUID)
		if err != nil {
			return empty, internalerrors.Capture(err)
		}
	}
	if !isPlaceholder(query) {
		var (
			retrievedBy     string
			retrievedByType coreresource.RetrievedByType
		)
		if target.unitUUID != "" {
			retrievedBy = target.unitUUID.String()
			retrievedByType = coreresource.Unit
		} else if userID != "" {
			retrievedBy = userID
			retrievedByType = coreresource.User
		} else {
			retrievedBy = target.appID.String()
			retrievedByType = coreresource.Application
		}

		err := resourceService.StoreResource(ctx, resource.StoreResourceArgs{
			ResourceUUID:    resUUID,
			Reader:          r.Body,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
		})
		if err != nil {
			return empty, internalerrors.Capture(err)
		}
	}

	return resourceService.GetResource(ctx, resUUID)
}

// isPlaceholder determines if the given query represents a placeholder by
// checking if the "timestamp" field is empty.
func isPlaceholder(query url.Values) bool {
	return query.Get("timestamp") == ""
}

type resourceUploadTarget struct {
	name     string
	appID    coreapplication.ID
	unitUUID coreunit.UUID
}

// getUploadTarget resolves the upload target by determining the application ID
// and optional unit UUID from the query inputs. It validates that either
// application or unit is specified, but not both, and fetches necessary details
// using the application service.
// Returns the name of the resource, the application ID and the unit UUID
// (if applicable), or an error if any issues occur during resolution.
func getUploadTarget(
	ctx context.Context,
	service ApplicationService,
	query url.Values,
) (target resourceUploadTarget, err error) {
	// Validate parameters
	target.name = query.Get("name")
	appName := query.Get("application")
	unitName := query.Get("unit")

	if target.name == "" {
		return target, errors.BadRequestf("missing resource name")
	}

	switch {
	case appName == "" && unitName == "":
		return target, errors.BadRequestf("missing application/unit")
	case appName != "" && unitName != "":
		return target, errors.BadRequestf("application and unit can't be set at the same time")
	}

	// Resolve target by unit name if any
	if unitName != "" {
		coreUnitName, err := coreunit.NewName(unitName)
		if err != nil {
			return target, errors.BadRequestf(err.Error())
		}
		target.unitUUID, err = service.GetUnitUUID(ctx, coreUnitName)
		if err != nil {
			return target, internalerrors.Capture(err)
		}
		target.appID, err = service.GetApplicationIDByUnitName(ctx, coreUnitName)
		return target, internalerrors.Capture(err)
	}

	// Resolve target by appName
	target.appID, err = service.GetApplicationIDByName(ctx, appName)
	return target, internalerrors.Capture(err)
}
