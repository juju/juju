// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

// A DataSource retrieves simplestreams metadata.
type DataSource interface {
	// Fetch loads the data at the specified relative path. It returns a reader from which
	// the data can be retrieved as well as the full URL of the file. The full URL is typically
	// used in log messages to help diagnose issues accessing the data.
	Fetch(path string) (io.ReadCloser, string, error)
	// URL returns the full URL of the path, as applicable to this datasource.
	// This method is used primarily for logging purposes.
	URL(path string) (string, error)
	// SetAllowRetry sets the flag which determines if the datasource will retry fetching the metadata
	// if it is not immediately available.
	SetAllowRetry(allow bool)
}

// SSLHostnameVerification is used as a switch for when a given provider might
// use self-signed credentials and we should not try to verify the hostname on
// the TLS/SSL certificates
type SSLHostnameVerification bool

const (
	// VerifySSLHostnames ensures we verify the hostname on the certificate
	// matches the host we are connecting and is signed
	VerifySSLHostnames = SSLHostnameVerification(true)
	// NoVerifySSLHostnames informs us to skip verifying the hostname
	// matches a valid certificate
	NoVerifySSLHostnames = SSLHostnameVerification(false)
)

// A urlDataSource retrieves data from an HTTP URL.
type urlDataSource struct {
	baseURL              string
	hostnameVerification SSLHostnameVerification
}

// NewURLDataSource returns a new datasource reading from the specified baseURL.
func NewURLDataSource(baseURL string, verify SSLHostnameVerification) DataSource {
	return &urlDataSource{
		baseURL:              baseURL,
		hostnameVerification: verify,
	}
}

func (u *urlDataSource) GoString() string {
	return fmt.Sprintf("urlDataSource(%q)", u.baseURL)
}

// urlJoin returns baseURL + relpath making sure to have a '/' inbetween them
// This doesn't try to do anything fancy with URL query or parameter bits
// It also doesn't use path.Join because that normalizes slashes, and you need
// to keep both slashes in 'http://'.
func urlJoin(baseURL, relpath string) string {
	if strings.HasSuffix(baseURL, "/") {
		return baseURL + relpath
	}
	return baseURL + "/" + relpath
}

// Fetch is defined in simplestreams.DataSource.
func (h *urlDataSource) Fetch(path string) (io.ReadCloser, string, error) {
	dataURL := urlJoin(h.baseURL, path)
	// dataURL can be http:// or file://
	client := http.DefaultClient
	if !h.hostnameVerification {
		client = utils.GetNonValidatingHTTPClient()
	}
	resp, err := client.Get(dataURL)
	if err != nil {
		logger.Debugf("Got error requesting %q: %v", dataURL, err)
		return nil, dataURL, errors.NotFoundf("invalid URL %q", dataURL)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, dataURL, errors.NotFoundf("cannot find URL %q", dataURL)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, dataURL, errors.Unauthorizedf("unauthorised access to URL %q", dataURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, dataURL, fmt.Errorf("cannot access URL %q, %q", dataURL, resp.Status)
	}
	return resp.Body, dataURL, nil
}

// URL is defined in simplestreams.DataSource.
func (h *urlDataSource) URL(path string) (string, error) {
	return urlJoin(h.baseURL, path), nil
}

// SetAllowRetry is defined in simplestreams.DataSource.
func (h *urlDataSource) SetAllowRetry(allow bool) {
	// This is a NOOP for url datasources.
}
