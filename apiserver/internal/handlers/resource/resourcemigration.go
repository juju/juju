// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/juju/errors"

	internalhttp "github.com/juju/juju/apiserver/internal/http"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	domainresource "github.com/juju/juju/domain/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// resourcesMigrationUploadHandler handles resources uploads for model migrations.
type resourcesMigrationUploadHandler struct {
	applicationServiceGetter ApplicationServiceGetter
	resourceServiceGetter    ResourceServiceGetter
	validator                Downloader
	logger                   logger.Logger
}

// NewResourceMigrationUploadHandler returns a new HTTP handler for resources
// uploads during model migrations.
func NewResourceMigrationUploadHandler(
	applicationServiceGetter ApplicationServiceGetter,
	resourceServiceGetter ResourceServiceGetter,
	validator Downloader,
	logger logger.Logger,
) *resourcesMigrationUploadHandler {
	return &resourcesMigrationUploadHandler{
		validator:                validator,
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
			h.logger.Errorf("cannot return error to user: %v", err)
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
) (resource.Resource, error) {
	var empty resource.Resource
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
		domainresource.GetApplicationResourceIDArgs{
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
			retrievedByType resource.RetrievedByType
		)
		if target.unitUUID != "" {
			retrievedBy = target.unitUUID.String()
			retrievedByType = resource.Unit
		} else if userID != "" {
			retrievedBy = userID
			retrievedByType = resource.User
		} else {
			retrievedBy = target.appID.String()
			retrievedByType = resource.Application
		}

		details, err := resourceDetailsFromQuery(query)
		if err != nil {
			return empty, internalerrors.Errorf("extracting resource details from request: %w", err)
		}

		reader, err := h.validator.Download(r.Body, details.fingerprint.String(), details.size)
		if err != nil {
			return empty, internalerrors.Errorf("validating resource size and hash: %w", err)
		}

		err = resourceService.StoreResource(ctx, domainresource.StoreResourceArgs{
			ResourceUUID:    resUUID,
			Reader:          reader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
			Size:            details.size,
			Fingerprint:     details.fingerprint,
			Origin:          details.origin,
			Revision:        details.revision,
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

type resourceDetails struct {
	// origin identifies where the resource will come from.
	origin charmresource.Origin
	// revision is the charm store revision of the resource.
	revision int
	// fingerprint is the SHA-384 checksum for the resource blob.
	fingerprint charmresource.Fingerprint
	// size is the size of the resource, in bytes.
	size int64
}

// resourceDetailsFromQuery extracts details about the uploaded resource from
// the request.
func resourceDetailsFromQuery(query url.Values) (resourceDetails, error) {
	var (
		details resourceDetails
		err     error
	)
	details.origin, err = charmresource.ParseOrigin(query.Get("origin"))
	if err != nil {
		return details, errors.BadRequestf("invalid origin")
	}
	details.revision, err = strconv.Atoi(query.Get("revision"))
	if err != nil {
		if details.origin == charmresource.OriginStore {
			return details, errors.BadRequestf("invalid revision")
		}
	}
	details.size, err = strconv.ParseInt(query.Get("size"), 10, 64)
	if err != nil {
		return details, errors.BadRequestf("invalid size")
	}
	details.fingerprint, err = charmresource.ParseFingerprint(query.Get("fingerprint"))
	if err != nil {
		return details, errors.BadRequestf("invalid fingerprint")
	}
	return details, nil
}
