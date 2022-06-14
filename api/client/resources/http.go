// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"fmt"
)

const (
	// HTTPEndpointPath is the URL path, with substitutions, for
	// a resource request.
	HTTPEndpointPath = "/applications/%s/resources/%s"
)

const (
	// ContentTypeRaw is the HTTP content-type value used for raw, unformattedcontent.
	ContentTypeRaw = "application/octet-stream"
)

const (
	// HeaderContentType is the header name for the type of a file upload.
	HeaderContentType = "Content-Type"
	// HeaderContentSha384 is the header name for the sha hash of a file upload.
	HeaderContentSha384 = "Content-Sha384"
	// HeaderContentLength is the header name for the length of a file upload.
	HeaderContentLength = "Content-Length"
	// HeaderContentDisposition is the header name for value that holds the filename.
	// The params are formatted according to  RFC 2045 and RFC 2616 (see
	// mime.ParseMediaType and mime.FormatMediaType).
	HeaderContentDisposition = "Content-Disposition"
)

const (
	// MediaTypeFormData is the media type for file uploads (see
	// mime.FormatMediaType).
	MediaTypeFormData = "form-data"
	// QueryParamPendingID is the query parameter we use to send up the pending id.
	QueryParamPendingID = "pendingid"
)

// newEndpointPath returns the API URL path for the identified resource.
func newEndpointPath(application, name string) string {
	return fmt.Sprintf(HTTPEndpointPath, application, name)
}
