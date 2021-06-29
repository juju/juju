// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

// TODO(ericsnow) Eliminate the apiserver dependencies, if possible.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.resource.api")

const (
	// HTTPEndpointPath is the URL path, with substitutions, for
	// a resource request.
	HTTPEndpointPath = "/applications/%s/resources/%s"
)

const (
	// ContentTypeRaw is the HTTP content-type value used for raw, unformattedcontent.
	ContentTypeRaw = "application/octet-stream"

	// ContentTypeJSON is the HTTP content-type value used for JSON content.
	ContentTypeJSON = "application/json"
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

// NewEndpointPath returns the API URL path for the identified resource.
func NewEndpointPath(application, name string) string {
	return fmt.Sprintf(HTTPEndpointPath, application, name)
}

// ExtractEndpointDetails pulls the endpoint wildcard values from
// the provided URL.
func ExtractEndpointDetails(url *url.URL) (application, name string) {
	application = url.Query().Get(":application")
	name = url.Query().Get(":resource")
	return application, name
}

// TODO(ericsnow) These are copied from apiserver/httpcontext.go...

// SendHTTPError sends a JSON-encoded error response
// for errors encountered during processing.
func SendHTTPError(w http.ResponseWriter, err error) {
	err1, statusCode := apiservererrors.ServerErrorAndStatus(err)
	logger.Debugf("sending error: %d %v", statusCode, err1)
	SendHTTPStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: err1,
	})
}

// SendHTTPStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func SendHTTPStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) {
	body, err := json.Marshal(response)
	if err != nil {
		http.Error(w, errors.Annotatef(err, "cannot marshal JSON result %#v", response).Error(), 504)
		return
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	_, err = w.Write(body)
	if err != nil {
		logger.Errorf("%v", err)
	}
}
