// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"io"
	"net/http"

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

	// GetPendingResource returns the identified resource.
	GetPendingResource(serviceID, name, pendingID string) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(serviceID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// UpdatePendingResource adds the resource to blob storage and updates the metadata.
	UpdatePendingResource(serviceID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)
}

// TODO(ericsnow) Replace UploadedResource with resource.Opened.

// UploadedResource holds both the information about an uploaded
// resource and the reader containing its data.
type UploadedResource struct {
	// Service is the name of the service associated with the resource.
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
func (uh UploadHandler) HandleRequest(req *http.Request) (*api.UploadResult, error) {
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

	result := &api.UploadResult{
		Resource: api.Resource2API(stored),
	}
	return result, nil
}

// ReadResource extracts the relevant info from the request.
func (uh UploadHandler) ReadResource(req *http.Request) (*UploadedResource, error) {
	uReq, err := api.ExtractUploadRequest(req)
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
