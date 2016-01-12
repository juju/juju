// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

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
	// TODO(ericsnow) Use the newer GenerateFingerprint()...
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := r.Seek(0, os.SEEK_SET); err != nil {
		return nil, errors.Trace(err)
	}

	fp, err := charmresource.GenerateFingerprint(data)
	if err != nil {
		return nil, errors.Trace(err)
	}

	method := "PUT"
	// TODO(ericsnow) What about the rest of the URL?
	urlStr := NewEndpointPath(service, name)
	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Sha384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.ContentLength = int64(len(data))

	return req, nil
}

// ExtractUploadRequest pulls the required info from the HTTP request.
func ExtractUploadRequest(req *http.Request) (service, name string, size int64, _ charmresource.Fingerprint, _ error) {
	var fp charmresource.Fingerprint

	if req.Header.Get("Content-Length") == "" {
		req.Header.Set("Content-Length", fmt.Sprint(req.ContentLength))
	}

	ctype := req.Header.Get("Content-Type")
	if ctype != "application/octet-stream" {
		return "", "", 0, fp, errors.Errorf("unsupported content type %q", ctype)
	}

	service, name = ExtractEndpointDetails(req.URL)

	fingerprint := req.Header.Get("Content-Sha384") // This parallels "Content-MD5".
	sizeRaw := req.Header.Get("Content-Length")

	// TODO(ericsnow) Use the newer ParseFingerprint().
	fpData, err := hex.DecodeString(fingerprint)
	if err != nil {
		return "", "", 0, fp, errors.Annotate(err, "invalid fingerprint")
	}
	fp, err = charmresource.NewFingerprint(fpData)
	if err != nil {
		return "", "", 0, fp, errors.Annotate(err, "invalid fingerprint")
	}

	size, err = strconv.ParseInt(sizeRaw, 10, 64)
	if err != nil {
		return "", "", 0, fp, errors.Annotate(err, "invalid size")
	}

	return service, name, size, fp, nil
}
