// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// A DataSource retrieves simplestreams metadata.
type DataSource interface {
	// Description describes the origin of this datasource.
	// eg agent-metadata-url, cloud storage, keystone catalog etc.
	Description() string

	// Fetch loads the data at the specified relative path. It returns a reader from which
	// the data can be retrieved as well as the full URL of the file. The full URL is typically
	// used in log messages to help diagnose issues accessing the data.
	Fetch(path string) (io.ReadCloser, string, error)

	// URL returns the full URL of the path, as applicable to this datasource.
	// This method is used primarily for logging purposes.
	URL(path string) (string, error)

	// PublicSigningKey returns the public key used to validate signed metadata.
	PublicSigningKey() string

	// SetAllowRetry sets the flag which determines if the datasource will retry fetching the metadata
	// if it is not immediately available.
	SetAllowRetry(allow bool)
}

// A urlDataSource retrieves data from an HTTP URL.
type urlDataSource struct {
	description          string
	baseURL              string
	hostnameVerification utils.SSLHostnameVerification
	publicSigningKey     string
}

// NewURLDataSource returns a new datasource reading from the specified baseURL.
func NewURLDataSource(description, baseURL string, hostnameVerification utils.SSLHostnameVerification) DataSource {
	return &urlDataSource{
		description:          description,
		baseURL:              baseURL,
		hostnameVerification: hostnameVerification,
	}
}

// NewURLSignedDataSource returns a new datasource for signed metadata reading from the specified baseURL.
func NewURLSignedDataSource(description, baseURL, publicKey string, hostnameVerification utils.SSLHostnameVerification) DataSource {
	return &urlDataSource{
		description:          description,
		baseURL:              baseURL,
		publicSigningKey:     publicKey,
		hostnameVerification: hostnameVerification,
	}
}

// Description is defined in simplestreams.DataSource.
func (u *urlDataSource) Description() string {
	return u.description
}

func (u *urlDataSource) GoString() string {
	return fmt.Sprintf("%v: urlDataSource(%q)", u.description, u.baseURL)
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
	client := utils.GetHTTPClient(h.hostnameVerification)
	// dataURL can be http:// or file://
	// MakeFileURL will only modify the URL if it's a file URL
	dataURL = utils.MakeFileURL(dataURL)
	resp, err := client.Get(dataURL)
	if err != nil {
		logger.Tracef("Got error requesting %q: %v", dataURL, err)
		return nil, dataURL, errors.NotFoundf("invalid URL %q", dataURL)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, dataURL, errors.NotFoundf("cannot find URL %q", dataURL)
		case http.StatusUnauthorized:
			return nil, dataURL, errors.Unauthorizedf("unauthorised access to URL %q", dataURL)
		}
		return nil, dataURL, fmt.Errorf("cannot access URL %q, %q", dataURL, resp.Status)
	}
	return resp.Body, dataURL, nil
}

// URL is defined in simplestreams.DataSource.
func (h *urlDataSource) URL(path string) (string, error) {
	return utils.MakeFileURL(urlJoin(h.baseURL, path)), nil
}

// PublicSigningKey is defined in simplestreams.DataSource.
func (u *urlDataSource) PublicSigningKey() string {
	return u.publicSigningKey
}

// SetAllowRetry is defined in simplestreams.DataSource.
func (h *urlDataSource) SetAllowRetry(allow bool) {
	// This is a NOOP for url datasources.
}
