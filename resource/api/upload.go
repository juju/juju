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

	// Filename is the name of the file as it exists on disk.
	Filename string

	// Size is the size of the uploaded data, in bytes.
	Size int64

	// Fingerprint is the fingerprint of the uploaded data.
	Fingerprint charmresource.Fingerprint

	// PendingID is the pending ID to associate with this upload, if any.
	PendingID string
}

// NewUploadRequest generates a new upload request for the given resource.
func NewUploadRequest(service, name, filename string, r io.ReadSeeker) (UploadRequest, error) {
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
		Filename:    filename,
		Size:        content.Size,
		Fingerprint: content.Fingerprint,
	}
	return ur, nil
}

// ExtractUploadRequest pulls the required info from the HTTP request.
func ExtractUploadRequest(req *http.Request) (UploadRequest, error) {
	var ur UploadRequest

	if req.Header.Get(HeaderContentLength) == "" {
		req.Header.Set(HeaderContentLength, fmt.Sprint(req.ContentLength))
	}

	ctype := req.Header.Get(HeaderContentType)
	if ctype != ContentTypeRaw {
		return ur, errors.Errorf("unsupported content type %q", ctype)
	}

	service, name := ExtractEndpointDetails(req.URL)
	fingerprint := req.Header.Get(HeaderContentSha384) // This parallels "Content-MD5".
	sizeRaw := req.Header.Get(HeaderContentLength)
	pendingID := req.URL.Query().Get(QueryParamPendingID)

	disp := req.Header.Get(HeaderContentDisposition)

	// the first value is the media type name (e.g. "form-data"), but we don't
	// really care.
	_, vals, err := parseMediaType(disp)
	if err != nil {
		return ur, errors.Annotate(err, "badly formatted Content-Disposition")
	}

	param, ok := vals[filenameParamForContentDispositionHeader]
	if !ok {
		return ur, errors.Errorf("missing filename in resource upload request")
	}

	filename, err := decodeParam(param)
	switch {
	case err == errInvalidWord:
		if encodeParam(param) == param {
			// The decode function doesn't differentiate between an invalid
			// string and a string that just didn't need to be encoded in the
			// first place.  So we run it through encode again to see if it's
			// just one of those "doesn't need to be encoded" strings.  This is
			// terrible, but I don't see a way around it.
			filename = param
			break
		}
		fallthrough
	case err != nil:
		return ur, errors.Annotatef(err, "couldn't decode filename %q from upload request", param)
	}

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
		Filename:    filename,
		Size:        size,
		Fingerprint: fp,
		PendingID:   pendingID,
	}
	return ur, nil
}

// filenameParamForContentDispositionHeader is the name of the parameter that
// contains the name of the file being uploaded, see mime.FormatMediaType and
// RFC 1867 (http://tools.ietf.org/html/rfc1867):
//
//   The original local file name may be supplied as well, either as a
//  'filename' parameter either of the 'content-disposition: form-data'
//   header or in the case of multiple files in a 'content-disposition:
//   file' header of the subpart.
const filenameParamForContentDispositionHeader = "filename"

// HTTPRequest generates a new HTTP request.
func (ur UploadRequest) HTTPRequest() (*http.Request, error) {
	// TODO(ericsnow) What about the rest of the URL?
	urlStr := NewEndpointPath(ur.Service, ur.Name)

	// TODO(natefinch): Use http.MethodPut when we upgrade to go1.5+.
	req, err := http.NewRequest(MethodPut, urlStr, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req.Header.Set(HeaderContentType, ContentTypeRaw)
	req.Header.Set(HeaderContentSha384, ur.Fingerprint.String())
	req.Header.Set(HeaderContentLength, fmt.Sprint(ur.Size))

	filename := encodeParam(ur.Filename)

	disp := formatMediaType(
		MediaTypeFormData,
		map[string]string{filenameParamForContentDispositionHeader: filename},
	)

	req.Header.Set(HeaderContentDisposition, disp)
	req.ContentLength = ur.Size

	if ur.PendingID != "" {
		query := req.URL.Query()
		query.Set(QueryParamPendingID, ur.PendingID)
		req.URL.RawQuery = query.Encode()
	}

	return req, nil
}

type encoder interface {
	Encode(charset, s string) string
}

type decoder interface {
	Decode(s string) (string, error)
}

func encodeParam(s string) string {
	return getEncoder().Encode("utf-8", s)
}

func decodeParam(s string) (string, error) {
	decoded, err := getDecoder().Decode(s)

	// If encoding is not required, the encoder will return the original string.
	// However, the decoder doesn't expect that, so it barfs on non-encoded
	// strings. To detect if a string was not encoded, we simply try encoding
	// again, if it returns the same string, we know it wasn't encoded.
	if err != nil && s == encodeParam(s) {
		return s, nil
	}
	return decoded, err
}
