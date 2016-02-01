// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"io"
	"net/http"
	"time"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// UploadDataStore describes the the portion of Juju's "state"
// needed for handling upload requests.
type UploadDataStore interface {
	// GetResource returns the identified resource.
	GetResource(serviceID, name string) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(serviceID string, res resource.Resource, r io.Reader) error
}

// TODO(ericsnow) Replace UploadedResource with resource.Opened.

// UploadedResource holds both the information about an uploaded
// resource and the reader containing its data.
type UploadedResource struct {
	// Service is the name of the service associated with the resource.
	Service string

	// Resource is the information about the resource.
	Resource resource.Resource

	// Data holds the resource blob.
	Data io.ReadCloser
}

// UploadHandler provides the functionality to handle upload requests.
type UploadHandler struct {
	// Username is the ID of the user making the upload request.
	Username string

	// Store is the data store into which the resource will be stored.
	Store UploadDataStore

	// CurrentTimestamp is the function that provides the current timestamp.
	CurrentTimestamp func() time.Time
}

// HandleRequest handles a resource upload request.
func (uh UploadHandler) HandleRequest(req *http.Request) (*api.UploadResult, error) {
	defer req.Body.Close()

	uploaded, err := uh.ReadResource(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	uploadID := uploaded.Service + "/" + uploaded.Resource.Name // TODO(ericsnow) Get this from state.
	if err := uh.Store.SetResource(uploaded.Service, uploaded.Resource, uploaded.Data); err != nil {
		return nil, errors.Trace(err)
	}

	result := &api.UploadResult{
		UploadID: uploadID,
	}
	return result, nil
}

// ReadResource extracts the relevant info from the request.
func (uh UploadHandler) ReadResource(req *http.Request) (*UploadedResource, error) {
	uReq, err := api.ExtractUploadRequest(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if uReq.PreUpload {
		// TODO(ericsnow) finish!
		return nil, errors.NotImplementedf("")
	}

	res, err := uh.Store.GetResource(uReq.Service, uReq.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	res, err = uh.updateResource(res, uReq.Fingerprint, uReq.Size)
	if err != nil {
		return nil, errors.Trace(err)
	}

	uploaded := &UploadedResource{
		Service:  uReq.Service,
		Resource: res,
		Data:     req.Body,
	}
	return uploaded, nil
}

// updateResource returns a copy of the provided resource, updated with
// the given information.
func (uh UploadHandler) updateResource(res resource.Resource, fp charmresource.Fingerprint, size int64) (resource.Resource, error) {
	res.Origin = charmresource.OriginUpload
	res.Revision = 0
	res.Fingerprint = fp
	res.Size = size
	res.Username = uh.Username
	res.Timestamp = uh.CurrentTimestamp().UTC()

	if err := res.Validate(); err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}
