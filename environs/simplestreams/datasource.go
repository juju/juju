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
	// Priority is an importance factor for Data Source. Higher number means higher priority.
	// This is will allow to sort data sources in order of importance.
	Priority() int
	// RequireSigned indicates whether this data source requires signed data.
	RequireSigned() bool
}

const (
	// These values used as priority factors for sorting data source data.

	// EXISTING_CLOUD_DATA is the lowest in priority.
	// It is mostly used in merge functions
	// where existing data does not need to be ranked.
	EXISTING_CLOUD_DATA = 0

	// DEFAULT_CLOUD_DATA is used for common cloud data that
	// is shared an is publically available.
	DEFAULT_CLOUD_DATA = 10

	// SPECIFIC_CLOUD_DATA is used to rank cloud specific data
	// above commonly available.
	// For e.g., openstack's "keystone catalogue".
	SPECIFIC_CLOUD_DATA = 20

	// CUSTOM_CLOUD_DATA is the highest available ranking and
	// is given to custom data.
	CUSTOM_CLOUD_DATA = 50
)

// A urlDataSource retrieves data from an HTTP URL.
type urlDataSource struct {
	description          string
	baseURL              string
	hostnameVerification utils.SSLHostnameVerification
	publicSigningKey     string
	priority             int
	requireSigned        bool
}

// NewURLDataSource returns a new datasource reading from the specified baseURL.
func NewURLDataSource(description, baseURL string, hostnameVerification utils.SSLHostnameVerification, priority int, requireSigned bool) DataSource {
	return &urlDataSource{
		description:          description,
		baseURL:              baseURL,
		hostnameVerification: hostnameVerification,
		priority:             priority,
		requireSigned:        requireSigned,
	}
}

// NewURLSignedDataSource returns a new datasource for signed metadata reading from the specified baseURL.
func NewURLSignedDataSource(description, baseURL, publicKey string, hostnameVerification utils.SSLHostnameVerification, priority int, requireSigned bool) DataSource {
	return &urlDataSource{
		description:          description,
		baseURL:              baseURL,
		publicSigningKey:     publicKey,
		hostnameVerification: hostnameVerification,
		priority:             priority,
		requireSigned:        requireSigned,
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

// Priority is defined in simplestreams.DataSource.
func (h *urlDataSource) Priority() int {
	return h.priority
}

// RequireSigned is defined in simplestreams.DataSource.
func (h *urlDataSource) RequireSigned() bool {
	return h.requireSigned
}
