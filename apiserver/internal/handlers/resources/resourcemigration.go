// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	internalhttp "github.com/juju/juju/apiserver/internal/http"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// resourcesMigrationUploadHandler handles resources uploads for model migrations.
type resourcesMigrationUploadHandler struct {
	resourceServiceGetter ResourceServiceGetter
	logger                logger.Logger
}

// NewResourceMigrationUploadHandler returns a new HTTP handler for resources
// uploads during model migrations.
func NewResourceMigrationUploadHandler(
	resourceServiceGetter ResourceServiceGetter,
	logger logger.Logger,
) *resourcesMigrationUploadHandler {
	return &resourcesMigrationUploadHandler{
		resourceServiceGetter: resourceServiceGetter,
		logger:                logger,
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
			h.logger.Errorf(r.Context(), "cannot return error to user: %v", err)
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

	res, err := h.processPost(r, resourceService)
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
) (coreresource.Resource, error) {
	var empty coreresource.Resource
	ctx := r.Context()
	query := r.URL.Query()
	resourceName := query.Get("name")
	if resourceName == "" {
		return empty, errors.BadRequestf("missing resource name")
	}
	if isPlaceholder(query) {
		// If the resource is a placeholder, do nothing. Information about
		// resources without an associated blob is also migrated during the
		// database migration, there is nothing to do here.
		return empty, nil
	}
	appName := query.Get("application")
	if appName == "" {
		// If the application name is empty, do nothing. Information about unit
		// resources is also migrated during the database migration. There is
		// nothing to do here.
		return empty, nil
	}

	// Get the resource UUID.
	resUUID, err := resourceService.GetResourceUUIDByApplicationAndResourceName(
		ctx, appName, resourceName,
	)
	if err != nil {
		return empty, internalerrors.Errorf(
			"resource upload failed: getting resource %s on application %s: %w",
			appName, resourceName, err,
		)
	}

	details, err := resourceDetailsFromQuery(query)
	if err != nil {
		return empty, internalerrors.Errorf("extracting resource details from request: %w", err)
	}

	// Check that the resource associated with the UUID has the expected Origin
	// and revision, to prevent updating the blob of another revision or origin
	// that may have been updated between the start of the http call and the
	// retrieval of resource UUID from application name and resource name.
	res, err := resourceService.GetResource(ctx, resUUID)
	if err != nil {
		return empty, internalerrors.Errorf("getting resource %s on application %s: %w", appName,
			resourceName, err)
	}
	if res.Origin != details.origin || (res.Origin != charmresource.OriginUpload && res.Revision != details.revision) {
		// In this case, we won't interrupt migration, because the user will
		// be able to recover after the migration. However, we log an error in
		// order to make it notice something get wrong and why.
		h.logger.Errorf(ctx, "resource upload failed: resource %s on application %s has unexpected origin or revision: %s != %s, %d != %d",
			appName, resourceName, res.Origin, details.origin, res.Revision, details.revision,
		)
		return res, nil
	}
	retrievedBy, retrievedByType := determineRetrievedBy(query)

	// Ideally we would verify that the hash and size of the blob in the request
	// body matches the hash and size in the headers. However, there is a bug
	// for container resources exported from 3.6 where the hash the header does
	// not match the hash in the body. For this reason, we do not check it here.

	err = resourceService.StoreResource(ctx, resource.StoreResourceArgs{
		ResourceUUID:    resUUID,
		Reader:          r.Body,
		RetrievedBy:     retrievedBy,
		RetrievedByType: retrievedByType,
		Size:            details.size,
		Fingerprint:     details.fingerprint,
	})
	if err != nil {
		return empty, internalerrors.Capture(err)
	}

	return resourceService.GetResource(ctx, resUUID)
}

// determineRetrievedBy determines the entity that retrieved the resource using
// the origin and user arguments on the query. If it cannot determine an entity,
// it returns the default values.
func determineRetrievedBy(query url.Values) (string, coreresource.RetrievedByType) {
	rawOrigin := query.Get("origin")
	origin, err := charmresource.ParseOrigin(rawOrigin)
	if err != nil {
		// If the origin cannot be determined, or is not present, return the
		// default values.
		return "", ""
	}

	// The user key contains the name of the entity that retrieved this
	// resource. Confusingly, this can be an application, unit or user.
	retrievedBy := query.Get("user")
	switch origin {
	case charmresource.OriginUpload:
		return retrievedBy, coreresource.User
	case charmresource.OriginStore:
		if names.IsValidUnit(retrievedBy) {
			return retrievedBy, coreresource.Unit
		}
		return retrievedBy, coreresource.Application
	default:
		return "", ""
	}
}

// isPlaceholder determines if the given query represents a placeholder by
// checking if the "timestamp" field is empty.
func isPlaceholder(query url.Values) bool {
	return query.Get("timestamp") == ""
}

// resourceDetails contains metadata about the resource from the request header.
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
// the query.
func resourceDetailsFromQuery(query url.Values) (resourceDetails, error) {
	var (
		details resourceDetails
		err     error
	)
	details.origin, err = charmresource.ParseOrigin(query.Get("origin"))
	if err != nil {
		return details, errors.BadRequestf("invalid origin: %w", err)
	}
	if details.origin == charmresource.OriginUpload {
		details.revision = -1
	} else {
		details.revision, err = strconv.Atoi(query.Get("revision"))
		if err != nil {
			return details, errors.BadRequestf("invalid revision: %w", err)
		}
	}
	details.size, err = strconv.ParseInt(query.Get("size"), 10, 64)
	if err != nil {
		return details, errors.BadRequestf("invalid size: %w", err)
	}
	details.fingerprint, err = charmresource.ParseFingerprint(query.Get("fingerprint"))
	if err != nil {
		return details, errors.BadRequestf("invalid fingerprint: %w", err)
	}
	return details, nil
}
