// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/utils/v3"

	corelogger "github.com/juju/juju/core/logger"
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
	// is shared an is publicly available.
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
	description      string
	baseURL          string
	publicSigningKey string
	priority         int
	requireSigned    bool
	httpClient       *jujuhttp.Client
}

// Config has values to be used in constructing a datasource.
type Config struct {
	// Description of the datasource
	Description string

	// BaseURL is the URL for this datasource.
	BaseURL string

	// HostnameVerification indicates whether to use self-signed credentials
	// and not try to verify the hostname on the TLS/SSL certificates.
	HostnameVerification bool

	// PublicSigningKey is the public key used to validate signed metadata.
	PublicSigningKey string

	// Priority is an importance factor for the datasource. Higher number means
	// higher priority. This is will facilitate sorting data sources in order of
	// importance.
	Priority int

	// RequireSigned indicates whether this datasource requires signed data.
	RequireSigned bool

	// CACertificates contains an optional list of Certificate
	// Authority certificates to be used to validate certificates
	// of cloud infrastructure components
	// The contents are Base64 encoded x.509 certs.
	CACertificates []string
}

// Validate checks that the baseURL is valid and the description is set.
func (c *Config) Validate() error {
	if c.Description == "" {
		return errors.New("no description specified")
	}
	if _, err := url.Parse(c.BaseURL); err != nil {
		return errors.Annotate(err, "base URL is not valid")
	}
	// TODO (hml) 2020-01-08
	// Add validation for PublicSigningKey
	return nil
}

// NewDataSource returns a new DataSource as defined
// by the given config.
func NewDataSource(cfg Config) DataSource {
	// TODO (hml) 2020-01-08
	// Move call to cfg.Validate() here and add return of error.
	client := jujuhttp.NewClient(
		jujuhttp.WithSkipHostnameVerification(!cfg.HostnameVerification),
		jujuhttp.WithCACertificates(cfg.CACertificates...),
		jujuhttp.WithLogger(logger.ChildWithLabels("http", corelogger.HTTP)),
	)
	return NewDataSourceWithClient(cfg, client)
}

// NewDataSourceWithClient returns a new DataSource as defines by the given
// Config, but with the addition of a http.Client.
func NewDataSourceWithClient(cfg Config, client *jujuhttp.Client) DataSource {
	return &urlDataSource{
		description:      cfg.Description,
		baseURL:          cfg.BaseURL,
		publicSigningKey: cfg.PublicSigningKey,
		priority:         cfg.Priority,
		requireSigned:    cfg.RequireSigned,
		httpClient:       client,
	}
}

// Description is defined in simplestreams.DataSource.
func (u *urlDataSource) Description() string {
	return u.description
}

func (u *urlDataSource) GoString() string {
	return fmt.Sprintf("%v: urlDataSource(%q)", u.description, u.baseURL)
}

// urlJoin returns baseURL + relpath making sure to have a '/' between them
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
	// MakeFileURL will only modify the URL if it's a file URL
	dataURL = utils.MakeFileURL(dataURL)
	resp, err := h.httpClient.Get(context.TODO(), dataURL)
	if err != nil {
		if !errors.IsNotFound(err) {
			// Callers of this mask the actual error.  Therefore warn here.
			// This is called multiple times when a machine is created, we
			// only need one success for images and one for tools.
			logger.Warningf("Got error requesting %q: %v", dataURL, err)
		}
		return nil, dataURL, errors.NewNotFound(err, fmt.Sprintf("%q", dataURL))
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, dataURL, errors.NotFoundf("%q", dataURL)
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

// Priority is defined in simplestreams.DataSource.
func (h *urlDataSource) Priority() int {
	return h.priority
}

// RequireSigned is defined in simplestreams.DataSource.
func (h *urlDataSource) RequireSigned() bool {
	return h.requireSigned
}
