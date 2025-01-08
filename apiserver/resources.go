// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"

	api "github.com/juju/juju/api/client/resources"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ResourceServiceGetter is an interface for getting an ResourceService.
type ResourceServiceGetter interface {
	// Resource returns the model's resource service.
	Resource(*http.Request) (ResourceService, error)
}

type resourceServiceGetter struct {
	ctxt httpContext
}

func (a *resourceServiceGetter) Resource(r *http.Request) (ResourceService, error) {
	domainServices, err := a.ctxt.domainServicesForRequest(r.Context())
	if err != nil {
		return nil, errors.Capture(err)
	}

	return domainServices.Resource(), nil
}

type ResourceService interface {
	// GetResourceUUIDByAppAndResourceName returns the ID of the application
	// resource specified by natural key of application and resource name.
	GetResourceUUIDByAppAndResourceName(
		ctx context.Context,
		appName string,
		resName string,
	) (coreresource.UUID, error)

	// GetResource returns the identified application resource.
	GetResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
	) (resource.Resource, error)

	// OpenResource returns the details of and a reader for the resource.
	//   - [resourceerrors.StoredResourceNotFound] if the specified resource is not
	//     in the resource store.
	OpenResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
	) (resource.Resource, io.ReadCloser, error)

	// StoreResource adds the application resource to blob storage and updates the
	// metadata. It also sets the retrival information for the resource.
	StoreResource(
		ctx context.Context,
		args resource.StoreResourceArgs,
	) error

	// GetApplicationResourceID returns the ID of the application resource
	// specified by natural key of application and resource name.
	GetApplicationResourceID(
		ctx context.Context,
		args resource.GetApplicationResourceIDArgs,
	) (coreresource.UUID, error)

	// SetUnitResource sets the resource metadata for a specific unit.
	SetUnitResource(
		ctx context.Context,
		resourceUUID coreresource.UUID,
		unitUUID coreunit.UUID,
	) error
}

// ResourcesBackend is the functionality of Juju's state needed for the resources API.
type ResourcesBackend interface {
	// OpenResource returns the identified resource and its content.
	OpenResource(applicationID, name string) (coreresource.Resource, io.ReadCloser, error)

	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (coreresource.Resource, error)

	// GetPendingResource returns the identified resource.
	GetPendingResource(applicationID, name, pendingID string) (coreresource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader, _ bool) (resource.Resource, error)

	// UpdatePendingResource adds the resource to blob storage and updates the metadata.
	UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)
}

// ResourcesHandler is the HTTP handler for client downloads and
// uploads of resources.
type ResourcesHandler struct {
	AuthFunc              func(*http.Request, ...string) (names.Tag, error)
	ChangeAllowedFunc     func(context.Context) error
	ResourceServiceGetter ResourceServiceGetter
}

// ServeHTTP implements http.Handler.
func (h *ResourcesHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	tag, err := h.AuthFunc(req, names.UserTagKind, names.MachineTagKind, names.ControllerAgentTagKind, names.ApplicationTagKind)
	if err != nil {
		if err := sendError(resp, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}

	resourceService, err := h.ResourceServiceGetter.Resource(req)
	if err != nil {
		if err := sendError(resp, err); err != nil {
			logger.Errorf("returning error to user: %v", err)
		}
		return
	}

	switch req.Method {
	case "GET":
		reader, size, err := h.download(resourceService, req)
		if err != nil {
			if err := sendError(resp, err); err != nil {
				logger.Errorf("%v", err)
			}
			return
		}
		defer reader.Close()
		header := resp.Header()
		header.Set("Content-Type", params.ContentTypeRaw)
		header.Set("Content-Length", fmt.Sprint(size))
		resp.WriteHeader(http.StatusOK)
		var bytesWritten int64
		if bytesWritten, err = io.Copy(resp, reader); err != nil {
			if err := sendError(resp, errors.Errorf("resource download failed: %w", err)); err != nil {
				logger.Errorf("returning error to user: %v", err)
			}
			return
		}
		if bytesWritten != size {
			logger.Errorf("resource download size does not match expected resource size: %d != %d", bytesWritten, size)
		}
	case "PUT":
		if err := h.ChangeAllowedFunc(req.Context()); err != nil {
			if err := sendError(resp, err); err != nil {
				logger.Errorf("returning error to user: %v", err)
			}
			return
		}
		response, err := h.upload(resourceService, req, tagToUsername(tag))
		if err != nil {
			if err := sendError(resp, err); err != nil {
				logger.Errorf("returning error to user: %v", err)
			}
			return
		}
		if err := sendStatusAndJSON(resp, http.StatusOK, &response); err != nil {
			logger.Errorf("sending response: %v", err)
		}
	default:
		if err := sendError(resp, jujuerrors.MethodNotAllowedf("unsupported method: %q", req.Method)); err != nil {
			logger.Errorf("returning error to user: %v", err)
		}
	}
}

func (h *ResourcesHandler) download(service ResourceService, req *http.Request) (io.ReadCloser, int64, error) {
	defer func() {
		if req.Body != nil {
			req.Body.Close()
		}
	}()

	query := req.URL.Query()
	application := query.Get(":application")
	name := query.Get(":resource")

	uuid, err := service.GetResourceUUIDByAppAndResourceName(req.Context(), application, name)
	if errors.Is(err, resourceerrors.ResourceNotFound) {
		return nil, 0, jujuerrors.NotFoundf("resource %s of application %s", name, application)
	} else if err != nil {
		return nil, 0, fmt.Errorf("getting resource uuid: %w", err)
	}

	res, reader, err := service.OpenResource(req.Context(), uuid)
	if errors.Is(err, resourceerrors.StoredResourceNotFound) {
		return nil, 0, jujuerrors.NotFoundf("resource %s of application %s has no blob downloaded on controller", name, application)
	} else if err != nil {
		return nil, 0, errors.Errorf("openeing resource %s for application %s: %w", name, application, err)
	}
	return reader, res.Size, err
}

func (h *ResourcesHandler) upload(resourceService ResourceService, req *http.Request, username string) (*params.UploadResult, error) {
	defer func() {
		if req.Body != nil {
			req.Body.Close()
		}
	}()

	uploaded, err := h.readResource(resourceService, req)
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = resourceService.StoreResource(req.Context(), resource.StoreResourceArgs{
		ResourceUUID:    uploaded.UUID,
		Reader:          uploaded.Data,
		RetrievedBy:     username,
		RetrievedByType: resource.User,
	})
	if err != nil {
		return nil, errors.Errorf("storing resource %s of application %s: %w", uploaded.Resource.Name, uploaded.Application, err)
	}

	return &params.UploadResult{
		Resource: api.DomainResource2API(uploaded.Resource),
	}, nil
}

// uploadedResource holds both the information about an uploaded
// resource and the reader containing its data.
type uploadedResource struct {
	// UUID is the resource UUID.
	UUID coreresource.UUID

	// Application is the name of the application associated with the resource.
	Application string

	// Resource is the information about the resource.
	Resource charmresource.Resource

	// Size is the size of the resource blob.
	Size int64

	// Fingerprint is the hash of the resource blob.
	Fingerprint charmresource.Fingerprint

	// Data holds the resource blob.
	Data io.ReadCloser
}

// readResource extracts the relevant info from the request.
func (h *ResourcesHandler) readResource(resourceService ResourceService, req *http.Request) (*uploadedResource, error) {
	uReq, err := extractUploadRequest(req)
	if err != nil {
		return nil, errors.Capture(err)
	}

	uuid, err := resourceService.GetResourceUUIDByAppAndResourceName(req.Context(), uReq.Application, uReq.Name)
	if errors.Is(err, resourceerrors.ResourceNotFound) {
		return nil, jujuerrors.NotFoundf("resource %s of application %s", uReq.Name, uReq.Application)
	} else if err != nil {
		return nil, errors.Errorf("getting resource uuid: %w", err)
	}

	res, err := resourceService.GetResource(req.Context(), uuid)
	if err != nil {
		return nil, errors.Errorf("getting resource details: %w", err)
	}

	switch res.Type {
	case charmresource.TypeFile:
		ext := path.Ext(res.Path)
		if path.Ext(uReq.Filename) != ext {
			return nil, errors.Errorf("incorrect extension on resource upload %q, expected %q", uReq.Filename, ext)
		}
	}

	// TODO(aflynn): Do we need to do this really?
	// No, but we do need to pass in the size and the fingerprint somehow.
	chRes, err := updateResource(res.Resource, uReq.Fingerprint, uReq.Size)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &uploadedResource{
		UUID:        res.UUID,
		Application: uReq.Application,
		Resource:    chRes,
		Data:        req.Body,
		Size:        uReq.Size,
		Fingerprint: uReq.Fingerprint,
	}, nil
}

// updateResource returns a copy of the provided resource, updated with
// the given information.
func updateResource(res charmresource.Resource, fp charmresource.Fingerprint, size int64) (charmresource.Resource, error) {
	res.Origin = charmresource.OriginUpload
	res.Revision = 0
	res.Fingerprint = fp
	res.Size = size

	if err := res.Validate(); err != nil {
		return res, errors.Capture(err)
	}
	return res, nil
}

// extractUploadRequest pulls the required info from the HTTP request.
func extractUploadRequest(req *http.Request) (api.UploadRequest, error) {
	var ur api.UploadRequest

	if req.Header.Get(api.HeaderContentLength) == "" {
		size := req.ContentLength
		// size will be negative if there is no content.
		if size < 0 {
			size = 0
		}
		req.Header.Set(api.HeaderContentLength, fmt.Sprint(size))
	}

	ctype := req.Header.Get(api.HeaderContentType)
	if ctype != api.ContentTypeRaw {
		return ur, errors.Errorf("unsupported content type %q", ctype)
	}

	application, name := extractEndpointDetails(req.URL)
	fingerprint := req.Header.Get(api.HeaderContentSha384) // This parallels "Content-MD5".
	sizeRaw := req.Header.Get(api.HeaderContentLength)

	fp, err := charmresource.ParseFingerprint(fingerprint)
	if err != nil {
		return ur, errors.Errorf("parsing fingerprint: %w", err)
	}

	filename, err := extractFilename(req)
	if err != nil {
		return ur, errors.Capture(err)
	}

	size, err := strconv.ParseInt(sizeRaw, 10, 64)
	if err != nil {
		return ur, errors.Errorf("parsing size: %w", err)
	}

	ur = api.UploadRequest{
		Application: application,
		Name:        name,
		Filename:    filename,
		Size:        size,
		Fingerprint: fp,
	}
	return ur, nil
}

// extractEndpointDetails pulls the endpoint wildcard values from
// the provided URL.
func extractEndpointDetails(url *url.URL) (application, name string) {
	application = url.Query().Get(":application")
	name = url.Query().Get(":resource")
	return application, name
}

func extractFilename(req *http.Request) (string, error) {
	disp := req.Header.Get(api.HeaderContentDisposition)

	// the first value returned here is the media type name (e.g. "form-data"),
	// but we don't really care.
	_, vals, err := mime.ParseMediaType(disp)
	if err != nil {
		return "", errors.Errorf("badly formatted Content-Disposition: %w", err)
	}

	param, ok := vals[api.FilenameParamForContentDispositionHeader]
	if !ok {
		return "", errors.Errorf("missing filename in resource upload request")
	}

	filename, err := decodeParam(param)
	if err != nil {
		return "", errors.Errorf("decoding filename %q from upload request: %w", param, err)
	}
	return filename, nil
}

func decodeParam(s string) (string, error) {
	decoded, err := new(mime.WordDecoder).Decode(s)

	// If encoding is not required, the encoder will return the original string.
	// However, the decoder doesn't expect that, so it barfs on non-encoded
	// strings. To detect if a string was not encoded, we simply try encoding
	// again, if it returns the same string, we know it wasn't encoded.
	if err != nil && s == encodeParam(s) {
		return s, nil
	}
	return decoded, err
}

func encodeParam(s string) string {
	return mime.BEncoding.Encode("utf-8", s)
}

func tagToUsername(tag names.Tag) string {
	switch tag := tag.(type) {
	case names.UserTag:
		return tag.Name()
	default:
		return ""
	}
}
