// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// UploadDataStore describes the the portion of Juju's "state"
// needed for handling upload requests.
type UploadDataStore interface {
	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (resource.Resource, error)

	// GetPendingResource returns the identified resource.
	GetPendingResource(applicationID, name, pendingID string) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// UpdatePendingResource adds the resource to blob storage and updates the metadata.
	UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)
}

// TODO(ericsnow) Replace UploadedResource with resource.Opened.

// UploadedResource holds both the information about an uploaded
// resource and the reader containing its data.
type UploadedResource struct {
	// Service is the name of the application associated with the resource.
	Service string

	// PendingID is the resource-specific sub-ID for a pending resource.
	PendingID string

	// Resource is the information about the resource.
	Resource charmresource.Resource

	// Data holds the resource blob.
	Data io.ReadCloser
}

// UploadHandler provides the functionality to handle upload requests.
type UploadHandler struct {
	// Username is the ID of the user making the upload request.
	Username string

	// Store is the data store into which the resource will be stored.
	Store UploadDataStore
}

// HandleRequest handles a resource upload request.
func (uh UploadHandler) HandleRequest(req *http.Request) (*params.UploadResult, error) {
	defer req.Body.Close()

	uploaded, err := uh.ReadResource(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var stored resource.Resource
	if uploaded.PendingID != "" {
		stored, err = uh.Store.UpdatePendingResource(uploaded.Service, uploaded.PendingID, uh.Username, uploaded.Resource, uploaded.Data)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		stored, err = uh.Store.SetResource(uploaded.Service, uh.Username, uploaded.Resource, uploaded.Data)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	result := &params.UploadResult{
		Resource: api.Resource2API(stored),
	}
	return result, nil
}

// ReadResource extracts the relevant info from the request.
func (uh UploadHandler) ReadResource(req *http.Request) (*UploadedResource, error) {
	uReq, err := extractUploadRequest(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var res resource.Resource
	if uReq.PendingID != "" {
		res, err = uh.Store.GetPendingResource(uReq.Service, uReq.Name, uReq.PendingID)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		res, err = uh.Store.GetResource(uReq.Service, uReq.Name)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	ext := path.Ext(res.Path)
	if path.Ext(uReq.Filename) != ext {
		return nil, errors.Errorf("incorrect extension on resource upload %q, expected %q", uReq.Filename, ext)
	}

	chRes, err := uh.updateResource(res.Resource, uReq.Fingerprint, uReq.Size)
	if err != nil {
		return nil, errors.Trace(err)
	}

	uploaded := &UploadedResource{
		Service:   uReq.Service,
		PendingID: uReq.PendingID,
		Resource:  chRes,
		Data:      req.Body,
	}
	return uploaded, nil
}

// updateResource returns a copy of the provided resource, updated with
// the given information.
func (uh UploadHandler) updateResource(res charmresource.Resource, fp charmresource.Fingerprint, size int64) (charmresource.Resource, error) {
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
		req.Header.Set(api.HeaderContentLength, fmt.Sprint(req.ContentLength))
	}

	ctype := req.Header.Get(api.HeaderContentType)
	if ctype != api.ContentTypeRaw {
		return ur, errors.Errorf("unsupported content type %q", ctype)
	}

	service, name := api.ExtractEndpointDetails(req.URL)
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
		Service:     service,
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
	_, vals, err := api.ParseMediaType(disp)
	if err != nil {
		return "", errors.Annotate(err, "badly formatted Content-Disposition")
	}

	param, ok := vals[api.FilenameParamForContentDispositionHeader]
	if !ok {
		return "", errors.Errorf("missing filename in resource upload request")
	}

	filename, err := api.DecodeParam(param)
	if err != nil {
		return "", errors.Annotatef(err, "couldn't decode filename %q from upload request", param)
	}
	return filename, nil
}
