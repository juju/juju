// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Eliminate the apiserver dependencies, if possible.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
)

var logger = loggo.GetLogger("juju.resource.api")

const (
	// HTTPEndpointPattern is the URL path pattern registered with
	// the API server. This includes wildcards (starting with ":") that
	// are converted into URL query values by the pattern mux. Also see
	// apiserver/apiserver.go.
	HTTPEndpointPattern = "/services/:service/resources/:resource"

	// HTTPEndpointPath is the URL path, with substitutions, for
	// a resource request.
	HTTPEndpointPath = "/services/%s/resources/%s"
)

const (
	// ContentTypeRaw is the HTTP content-type value used for raw, unformattedcontent.
	ContentTypeRaw = "application/octet-stream"

	// ContentTypeJSON is the HTTP content-type value used for JSON content.
	ContentTypeJSON = "application/json"
)

// NewEndpointPath returns the API URL path for the identified resource.
func NewEndpointPath(service, name string) string {
	return fmt.Sprintf(HTTPEndpointPath, service, name)
}

// ExtractEndpointDetails pulls the endpoint wildcard values from
// the provided URL.
func ExtractEndpointDetails(url *url.URL) (service, name string) {
	service = url.Query().Get(":service")
	name = url.Query().Get(":resource")
	return service, name
}

// NewHTTPUploadRequest generates a new HTTP request for the given resource.
func NewHTTPUploadRequest(service, name string, r io.ReadSeeker) (*http.Request, error) {
	sizer := utils.NewSizingReader(r)
	fp, err := charmresource.GenerateFingerprint(sizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := r.Seek(0, os.SEEK_SET); err != nil {
		return nil, errors.Trace(err)
	}
	size := sizer.Size()

	method := "PUT"
	// TODO(ericsnow) What about the rest of the URL?
	urlStr := NewEndpointPath(service, name)
	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req.Header.Set("Content-Type", ContentTypeRaw)
	req.Header.Set("Content-Sha384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(size))
	req.ContentLength = size

	return req, nil
}

// ExtractUploadRequest pulls the required info from the HTTP request.
func ExtractUploadRequest(req *http.Request) (service, name string, size int64, _ charmresource.Fingerprint, _ error) {
	var fp charmresource.Fingerprint

	if req.Header.Get("Content-Length") == "" {
		req.Header.Set("Content-Length", fmt.Sprint(req.ContentLength))
	}

	ctype := req.Header.Get("Content-Type")
	if ctype != ContentTypeRaw {
		return "", "", 0, fp, errors.Errorf("unsupported content type %q", ctype)
	}

	service, name = ExtractEndpointDetails(req.URL)

	fingerprint := req.Header.Get("Content-Sha384") // This parallels "Content-MD5".
	sizeRaw := req.Header.Get("Content-Length")

	fp, err := charmresource.ParseFingerprint(fingerprint)
	if err != nil {
		return "", "", 0, fp, errors.Annotate(err, "invalid fingerprint")
	}

	size, err = strconv.ParseInt(sizeRaw, 10, 64)
	if err != nil {
		return "", "", 0, fp, errors.Annotate(err, "invalid size")
	}

	return service, name, size, fp, nil
}

// NewHTTPDownloadRequest creates a new HTTP download request
// for the given resource.
//
// Intended for use on the client side.
func NewHTTPDownloadRequest(resourceName string) (*http.Request, error) {
	return http.NewRequest("GET", "/resources/"+resourceName, nil)
}

// ExtractDownloadRequest pulls the download request info out of the
// given HTTP request.
//
// Intended for use on the server side.
func ExtractDownloadRequest(req *http.Request) string {
	return req.URL.Query().Get(":resource")
}

// UpdateDownloadResponse sets the appropriate headers in the response
// to an HTTP download request.
//
// Intended for use on the server side.
func UpdateDownloadResponse(resp http.ResponseWriter, resource resource.Resource) {
	resp.Header().Set("Content-Type", ContentTypeRaw)
	resp.Header().Set("Content-Length", fmt.Sprint(resource.Size))
	resp.Header().Set("Content-Sha384", resource.Fingerprint.String())
}

// ExtractDownloadResponse pulls the download size and checksum
// from the HTTP response.
func ExtractDownloadResponse(resp *http.Response) (int64, charmresource.Fingerprint, error) {
	var fp charmresource.Fingerprint

	// TODO(ericsnow) Finish!
	return 0, fp, errors.New("not finished")
}

// TODO(ericsnow) These are copied from apiserver/httpcontext.go...

// SendHTTPError sends a JSON-encoded error response
// for errors encountered during processing.
func SendHTTPError(w http.ResponseWriter, err error) {
	err1, statusCode := common.ServerErrorAndStatus(err)
	logger.Debugf("sending error: %d %v", statusCode, err1)
	SendHTTPStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: err1,
	})
}

// SendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func SendHTTPStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) {
	body, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("cannot marshal JSON result %#v: %v", response, err)
		return
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	w.Write(body)
}
