// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/docker/distribution/reference"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/docker"
)

var logger = loggo.GetLogger("juju.docker.registry.internal")

const (
	defaultTimeout = 15 * time.Second
)

// APIVersion is the API version type.
type APIVersion string

const (
	// APIVersionV1 is the API version v1.
	APIVersionV1 APIVersion = "v1"
	// APIVersionV2 is the API version v2.
	APIVersionV2 APIVersion = "v2"
)

func (v APIVersion) String() string {
	return string(v)
}

type baseClient struct {
	baseURL     *url.URL
	client      *http.Client
	repoDetails *docker.ImageRepoDetails
}

func newBase(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) *baseClient {
	c := &baseClient{
		baseURL:     &url.URL{},
		repoDetails: &repoDetails,
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
	}
	c.prepare()
	return c
}

// prepare does pre-processing before Match().
func (c *baseClient) prepare() {
	if c.repoDetails.ServerAddress != "" {
		return
	}
	// We have validated the repository in top level.
	// It should not raise errors here.
	named, _ := reference.ParseNormalizedNamed(c.repoDetails.Repository)
	domain := reference.Domain(named)
	if domain != "" {
		c.repoDetails.ServerAddress = domain
	}
}

// Match checks if the repository details matches current provider format.
func (c *baseClient) Match() bool {
	return false
}

// APIVersion returns the registry API version to use.
func (c *baseClient) APIVersion() APIVersion {
	if c.repoDetails.IsPrivate() {
		return APIVersionV2
	}
	return APIVersionV1
}

// TransportWrapper wraps RoundTripper then return.
type TransportWrapper func(http.RoundTripper, *docker.ImageRepoDetails) (http.RoundTripper, error)

func transportCommon(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails) (http.RoundTripper, error) {
	if repoDetails.IsPrivate() {
		if !repoDetails.BasicAuthConfig.Empty() {
			transport = newTokenTransport(
				transport, repoDetails.Username, repoDetails.Password, repoDetails.Auth, "", false,
			)
		}
		if !repoDetails.TokenAuthConfig.Empty() {
			return nil, errors.New(
				fmt.Sprintf(
					`only "username" and "password" or "auth" token authorization is supported for registry %q`,
					repoDetails.ServerAddress,
				),
			)
		}
	}
	return transport, nil
}

func wrapTransport(
	transport http.RoundTripper,
	repoDetails *docker.ImageRepoDetails,
	wrappers ...TransportWrapper,
) (http.RoundTripper, error) {
	var err error
	for _, wrap := range wrappers {
		if transport, err = wrap(transport, repoDetails); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return transport, nil
}

func wrapErrorTransport(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails) (http.RoundTripper, error) {
	return newErrorTransport(transport), nil
}

func (c *baseClient) WrapTransport(wrappers ...TransportWrapper) (err error) {
	wrappers = append(wrappers, transportCommon, wrapErrorTransport)
	if c.client.Transport, err = wrapTransport(c.client.Transport, c.repoDetails, wrappers...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func decideBaseURLCommon(version APIVersion, repoDetails *docker.ImageRepoDetails, baseURL *url.URL) error {
	addr := repoDetails.ServerAddress
	if addr == "" {
		return errors.NotValidf("empty server address for %q", repoDetails.Repository)
	}
	url, err := url.Parse(addr)
	if err != nil {
		return errors.Annotatef(err, "parsing server address %q", addr)
	}
	serverAddressURL := *url
	apiVersion := version.String()
	if !strings.Contains(url.Path, "/"+apiVersion) {
		url.Path = path.Join(url.Path, apiVersion)
	}
	if url.Scheme == "" {
		url.Scheme = "https"
	}
	*baseURL = *url

	serverAddressURL.Scheme = ""
	repoDetails.ServerAddress = serverAddressURL.String()
	logger.Tracef("baseClient repoDetails %#v", repoDetails)
	return nil
}

// DecideBaseURL decides the API url to use.
func (c *baseClient) DecideBaseURL() error {
	return errors.Trace(decideBaseURLCommon(c.APIVersion(), c.repoDetails, c.baseURL))
}

func commonURL(version APIVersion, url url.URL, pathTemplate string, args ...interface{}) string {
	pathSuffix := fmt.Sprintf(pathTemplate, args...)
	ver := version.String()
	if !strings.HasSuffix(strings.TrimRight(url.Path, "/"), ver) {
		url.Path = path.Join(url.Path, ver)
	}
	if url.Scheme == "" {
		url.Scheme = "https"
	}
	url.Path = path.Join(url.Path, pathSuffix)
	return url.String()
}

func (c baseClient) url(pathTemplate string, args ...interface{}) string {
	return commonURL(c.APIVersion(), *c.baseURL, pathTemplate, args...)
}

// Ping pings the baseClient endpoint.
func (c baseClient) Ping() error {
	url := c.url("/")
	logger.Debugf("baseClient ping %q", url)
	resp, err := c.client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	return errors.Trace(err)
}

func (c baseClient) ImageRepoDetails() (o docker.ImageRepoDetails) {
	if c.repoDetails != nil {
		return *c.repoDetails
	}
	return o
}

// Close closes the transport used by the client.
func (c *baseClient) Close() error {
	if t, ok := c.client.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
	return nil
}

func (c baseClient) getPaginatedJSON(url string, response interface{}) (string, error) {
	resp, err := c.client.Get(url)
	logger.Tracef("getPaginatedJSON for %q, err %v", url, err)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(response)
	if err != nil {
		return "", errors.Trace(err)
	}
	return getNextLink(resp)
}

var (
	nextLinkRE     = regexp.MustCompile(`^ *<?([^;>]+)>? *(?:;[^;]*)*; *rel="?next"?(?:;.*)?`)
	errNoMorePages = errors.New("no more pages")
)

func getNextLink(resp *http.Response) (string, error) {
	for _, link := range resp.Header[http.CanonicalHeaderKey("Link")] {
		parts := nextLinkRE.FindStringSubmatch(link)
		if parts != nil {
			return parts[1], nil
		}
	}
	return "", errNoMorePages
}
