// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/state"
)

// ResourcesBackend is the functionality of Juju's state needed for the resources API.
type ResourcesBackend interface {
	// OpenResource returns the identified resource and its content.
	OpenResource(applicationID, name string) (resource.Resource, io.ReadCloser, error)

	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (resource.Resource, error)

	// GetPendingResource returns the identified resource.
	GetPendingResource(applicationID, name, pendingID string) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// UpdatePendingResource adds the resource to blob storage and updates the metadata.
	UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)
}

// ResourcesHandler is the HTTP handler for client downloads and
// uploads of resources.
type ResourcesHandler struct {
	StateAuthFunc     func(*http.Request, ...string) (ResourcesBackend, state.PoolHelper, names.Tag, error)
	ChangeAllowedFunc func(*http.Request) error
}

// ServeHTTP implements http.Handler.
func (h *ResourcesHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	backend, poolhelper, tag, err := h.StateAuthFunc(req, names.UserTagKind, names.MachineTagKind, names.ControllerAgentTagKind, names.ApplicationTagKind)
	if err != nil {
		api.SendHTTPError(resp, err)
		return
	}
	defer poolhelper.Release()

	switch req.Method {
	case "GET":
		reader, size, err := h.download(backend, req)
		if err != nil {
			api.SendHTTPError(resp, err)
			return
		}
		defer reader.Close()
		header := resp.Header()
		header.Set("Content-Type", params.ContentTypeRaw)
		header.Set("Content-Length", fmt.Sprint(size))
		resp.WriteHeader(http.StatusOK)
		if _, err := io.Copy(resp, reader); err != nil {
			logger.Errorf("resource download failed: %v", err)
		}
	case "PUT":
		if err := h.ChangeAllowedFunc(req); err != nil {
			api.SendHTTPError(resp, err)
			return
		}
		response, err := h.upload(backend, req, tagToUsername(tag))
		if err != nil {
			api.SendHTTPError(resp, err)
			return
		}
		api.SendHTTPStatusAndJSON(resp, http.StatusOK, &response)
	default:
		api.SendHTTPError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
	}
}

func (h *ResourcesHandler) download(backend ResourcesBackend, req *http.Request) (io.ReadCloser, int64, error) {
	defer req.Body.Close()

	query := req.URL.Query()
	application := query.Get(":application")
	name := query.Get(":resource")

	resource, reader, err := backend.OpenResource(application, name)
	return reader, resource.Size, errors.Trace(err)
}

func (h *ResourcesHandler) upload(backend ResourcesBackend, req *http.Request, username string) (*params.UploadResult, error) {
	defer req.Body.Close()

	uploaded, err := h.readResource(backend, req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// UpdatePendingResource does the same as SetResource (just calls setResource) except SetResouce just blanks PendingID.
	var stored resource.Resource
	if uploaded.PendingID != "" {
		stored, err = backend.UpdatePendingResource(uploaded.Application, uploaded.PendingID, username, uploaded.Resource, uploaded.Data)
	} else {
		stored, err = backend.SetResource(uploaded.Application, username, uploaded.Resource, uploaded.Data)
	}

	if err != nil {
		return nil, errors.Trace(err)
	}

	result := &params.UploadResult{
		Resource: api.Resource2API(stored),
	}
	return result, nil
}

// uploadedResource holds both the information about an uploaded
// resource and the reader containing its data.
type uploadedResource struct {
	// Application is the name of the application associated with the resource.
	Application string

	// PendingID is the resource-specific sub-ID for a pending resource.
	PendingID string

	// Resource is the information about the resource.
	Resource charmresource.Resource

	// Data holds the resource blob.
	Data io.ReadCloser
}

// readResource extracts the relevant info from the request.
func (h *ResourcesHandler) readResource(backend ResourcesBackend, req *http.Request) (*uploadedResource, error) {
	uReq, err := extractUploadRequest(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var res resource.Resource
	if uReq.PendingID != "" {
		res, err = backend.GetPendingResource(uReq.Application, uReq.Name, uReq.PendingID)
	} else {
		res, err = backend.GetResource(uReq.Application, uReq.Name)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	switch res.Type {
	case charmresource.TypeFile:
		ext := path.Ext(res.Path)
		if path.Ext(uReq.Filename) != ext {
			return nil, errors.Errorf("incorrect extension on resource upload %q, expected %q", uReq.Filename, ext)
		}
	}

	chRes, err := updateResource(res.Resource, uReq.Fingerprint, uReq.Size)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &uploadedResource{
		Application: uReq.Application,
		PendingID:   uReq.PendingID,
		Resource:    chRes,
		Data:        req.Body,
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
		return res, errors.Trace(err)
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

	application, name := api.ExtractEndpointDetails(req.URL)
	fingerprint := req.Header.Get(api.HeaderContentSha384) // This parallels "Content-MD5".
	sizeRaw := req.Header.Get(api.HeaderContentLength)
	pendingID := req.URL.Query().Get(api.QueryParamPendingID)

	fp, err := charmresource.ParseFingerprint(fingerprint)
	if err != nil {
		return ur, errors.Annotate(err, "invalid fingerprint")
	}

	filename, err := extractFilename(req)
	if err != nil {
		return ur, errors.Trace(err)
	}

	size, err := strconv.ParseInt(sizeRaw, 10, 64)
	if err != nil {
		return ur, errors.Annotate(err, "invalid size")
	}

	ur = api.UploadRequest{
		Application: application,
		Name:        name,
		Filename:    filename,
		Size:        size,
		Fingerprint: fp,
		PendingID:   pendingID,
	}
	return ur, nil
}

func extractFilename(req *http.Request) (string, error) {
	disp := req.Header.Get(api.HeaderContentDisposition)

	// the first value returned here is the media type name (e.g. "form-data"),
	// but we don't really care.
	_, vals, err := mime.ParseMediaType(disp)
	if err != nil {
		return "", errors.Annotate(err, "badly formatted Content-Disposition")
	}

	param, ok := vals[api.FilenameParamForContentDispositionHeader]
	if !ok {
		return "", errors.Errorf("missing filename in resource upload request")
	}

	filename, err := decodeParam(param)
	if err != nil {
		return "", errors.Annotatef(err, "couldn't decode filename %q from upload request", param)
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
