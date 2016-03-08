// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

// UploadRequest defines a single upload request.
type UploadRequest struct {
	// Service is the service ID.
	Service string

	// Name is the resource name.
	Name string

	// Size is the size of the uploaded data, in bytes.
	Size int64

	// Fingerprint is the fingerprint of the uploaded data.
	Fingerprint charmresource.Fingerprint

	// PendingID is the pending ID to associate with this upload, if any.
	PendingID string
}

// NewUploadRequest generates a new upload request for the given resource.
func NewUploadRequest(service, name string, r io.ReadSeeker) (UploadRequest, error) {
	if !names.IsValidService(service) {
		return UploadRequest{}, errors.Errorf("invalid service %q", service)
	}

	content, err := resource.GenerateContent(r)
	if err != nil {
		return UploadRequest{}, errors.Trace(err)
	}

	ur := UploadRequest{
		Service:     service,
		Name:        name,
		Size:        content.Size,
		Fingerprint: content.Fingerprint,
	}
	return ur, nil
}

// ExtractUploadRequest pulls the required info from the HTTP request.
func ExtractUploadRequest(req *http.Request) (UploadRequest, error) {
	var ur UploadRequest

	if req.Header.Get("Content-Length") == "" {
		req.Header.Set("Content-Length", fmt.Sprint(req.ContentLength))
	}

	ctype := req.Header.Get("Content-Type")
	if ctype != ContentTypeRaw {
		return ur, errors.Errorf("unsupported content type %q", ctype)
	}

	service, name := ExtractEndpointDetails(req.URL)
	fingerprint := req.Header.Get("Content-Sha384") // This parallels "Content-MD5".
	sizeRaw := req.Header.Get("Content-Length")
	pendingID := req.URL.Query().Get("pendingid")

	fp, err := charmresource.ParseFingerprint(fingerprint)
	if err != nil {
		return ur, errors.Annotate(err, "invalid fingerprint")
	}

	size, err := strconv.ParseInt(sizeRaw, 10, 64)
	if err != nil {
		return ur, errors.Annotate(err, "invalid size")
	}

	ur = UploadRequest{
		Service:     service,
		Name:        name,
		Size:        size,
		Fingerprint: fp,
		PendingID:   pendingID,
	}
	return ur, nil
}

// HTTPRequest generates a new HTTP request.
func (ur UploadRequest) HTTPRequest() (*http.Request, error) {
	// TODO(ericsnow) What about the rest of the URL?
	urlStr := NewEndpointPath(ur.Service, ur.Name)
	req, err := http.NewRequest("PUT", urlStr, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req.Header.Set("Content-Type", ContentTypeRaw)
	req.Header.Set("Content-Sha384", ur.Fingerprint.String())
	req.Header.Set("Content-Length", fmt.Sprint(ur.Size))
	req.ContentLength = ur.Size

	if ur.PendingID != "" {
		query := req.URL.Query()
		query.Set("pendingid", ur.PendingID)
		req.URL.RawQuery = query.Encode()
	}

	return req, nil
}
