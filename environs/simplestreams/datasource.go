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
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/v4"

	corelogger "github.com/juju/juju/core/logger"
	jujuhttp "github.com/juju/juju/internal/http"
)

// A DataSource retrieves simplestreams metadata.
type DataSource interface {
	// Description describes the origin of this datasource.
	// eg agent-metadata-url, cloud storage, keystone catalog etc.
	Description() string

	// Fetch loads the data at the specified relative path. It returns a reader from which
	// the data can be retrieved as well as the full URL of the file. The full URL is typically
	// used in log messages to help diagnose issues accessing the data.
	Fetch(ctx context.Context, path string) (io.ReadCloser, string, error)

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
	clock            clock.Clock
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

	// Clock is used for retry. Will use clock.WallClock if nil.
	Clock clock.Clock
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
		jujuhttp.WithLogger(logger.Child("http", corelogger.HTTP)),
	)
	return NewDataSourceWithClient(cfg, client)
}

// NewDataSourceWithClient returns a new DataSource as defines by the given
// Config, but with the addition of a http.Client.
func NewDataSourceWithClient(cfg Config, client *jujuhttp.Client) DataSource {
	clk := cfg.Clock
	if clk == nil {
		clk = clock.WallClock
	}
	return &urlDataSource{
		description:      cfg.Description,
		baseURL:          cfg.BaseURL,
		publicSigningKey: cfg.PublicSigningKey,
		priority:         cfg.Priority,
		requireSigned:    cfg.RequireSigned,
		httpClient:       client,
		clock:            clk,
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
func (h *urlDataSource) Fetch(ctx context.Context, path string) (io.ReadCloser, string, error) {
	var readCloser io.ReadCloser
	dataURL := urlJoin(h.baseURL, path)
	// dataURL can be http:// or file://
	// MakeFileURL will only modify the URL if it's a file URL
	dataURL = utils.MakeFileURL(dataURL)

	err := retry.Call(retry.CallArgs{
		Func: func() error {
			var err error
			readCloser, err = h.fetch(ctx, dataURL)
			return err
		},
		IsFatalError: func(err error) bool {
			return errors.Is(err, errors.NotFound) || errors.Is(err, errors.Unauthorized)
		},
		Attempts:    3,
		Delay:       time.Second,
		MaxDelay:    time.Second * 5,
		BackoffFunc: retry.DoubleDelay,
		Clock:       h.clock,
	})
	return readCloser, dataURL, err
}

func (h *urlDataSource) fetch(ctx context.Context, path string) (io.ReadCloser, error) {
	resp, err := h.httpClient.Get(ctx, path)
	if err != nil {
		// Callers of this mask the actual error.  Therefore warn here.
		// This is called multiple times when a machine is created, we
		// only need one success for images and one for tools.
		logger.Warningf(ctx, "Got error requesting %q: %v", path, err)
		return nil, fmt.Errorf("cannot access URL %q: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, errors.NotFoundf("%q", path)
		case http.StatusUnauthorized:
			return nil, errors.Unauthorizedf("unauthorised access to URL %q", path)
		}
		return nil, fmt.Errorf("cannot access URL %q, %q", path, resp.Status)
	}
	return resp.Body, nil
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
