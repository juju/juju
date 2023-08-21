// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"fmt"
	"io"
	"mime"
	"net/http"

	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/resources"
)

// UploadRequest defines a single upload request.
type UploadRequest struct {
	// Application is the application ID.
	Application string

	// Name is the resource name.
	Name string

	// Filename is the name of the file as it exists on disk.
	Filename string

	// Size is the size of the uploaded data, in bytes.
	Size int64

	// Fingerprint is the fingerprint of the uploaded data.
	Fingerprint charmresource.Fingerprint

	// PendingID is the pending ID to associate with this upload, if any.
	PendingID string

	// Content is the content to upload.
	Content io.ReadSeeker
}

// NewUploadRequest generates a new upload request for the given resource.
func NewUploadRequest(application, name, filename string, r io.ReadSeeker) (UploadRequest, error) {
	if !names.IsValidApplication(application) {
		return UploadRequest{}, errors.Errorf("invalid application %q", application)
	}

	content, err := resources.GenerateContent(r)
	if err != nil {
		return UploadRequest{}, errors.Trace(err)
	}

	ur := UploadRequest{
		Application: application,
		Name:        name,
		Filename:    filename,
		Size:        content.Size,
		Fingerprint: content.Fingerprint,
		Content:     r,
	}
	return ur, nil
}

func setFilename(filename string, req *http.Request) {
	filename = mime.BEncoding.Encode("utf-8", filename)

	disp := mime.FormatMediaType(
		MediaTypeFormData,
		map[string]string{FilenameParamForContentDispositionHeader: filename},
	)

	req.Header.Set(HeaderContentDisposition, disp)
}

// FilenameParamForContentDispositionHeader is the name of the parameter that
// contains the name of the file being uploaded, see mime.FormatMediaType and
// RFC 1867 (http://tools.ietf.org/html/rfc1867):
//
//	 The original local file name may be supplied as well, either as a
//	'filename' parameter either of the 'content-disposition: form-data'
//	 header or in the case of multiple files in a 'content-disposition:
//	 file' header of the subpart.
const FilenameParamForContentDispositionHeader = "filename"

// HTTPRequest generates a new HTTP request.
func (ur UploadRequest) HTTPRequest() (*http.Request, error) {
	// TODO(ericsnow) What about the rest of the URL?
	urlStr := newEndpointPath(ur.Application, ur.Name)

	req, err := http.NewRequest(http.MethodPut, urlStr, ur.Content)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req.Header.Set(HeaderContentType, ContentTypeRaw)
	req.Header.Set(HeaderContentSha384, ur.Fingerprint.String())
	req.Header.Set(HeaderContentLength, fmt.Sprint(ur.Size))
	setFilename(ur.Filename, req)

	req.ContentLength = ur.Size

	if ur.PendingID != "" {
		query := req.URL.Query()
		query.Set(QueryParamPendingID, ur.PendingID)
		req.URL.RawQuery = query.Encode()
	}

	return req, nil
}
