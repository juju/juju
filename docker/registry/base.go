// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/docker"
)

var logger = loggo.GetLogger("juju.docker.registry")

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

var (
	// Override for testing.
	DefaultTransport = http.DefaultTransport
)

type baseClient struct {
	baseURL     *url.URL
	client      *http.Client
	repoDetails *docker.ImageRepoDetails
}

func newBase(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) *baseClient {
	c := &baseClient{
		repoDetails: &repoDetails,
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
	}
	return c
}

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

func (c *baseClient) WrapTransport() error {
	logger.Criticalf("baseClient.WrapTransport")
	transport := c.client.Transport
	if c.repoDetails.IsPrivate() {
		if !c.repoDetails.BasicAuthConfig.Empty() {
			transport = newTokenTransport(
				transport, c.repoDetails.Username, c.repoDetails.Password, c.repoDetails.Auth, "",
			)
		}
		if !c.repoDetails.TokenAuthConfig.Empty() {
			return errors.New(
				fmt.Sprintf(
					`only "username" and "password" or "auth" token authorization is supported for registry %q`,
					c.repoDetails.ServerAddress,
				),
			)
		}
	}
	c.client.Transport = newErrorTransport(transport)
	return nil
}

func (c *baseClient) DecideBaseURL() error {
	logger.Criticalf("baseClient.DecideBaseURL")
	addr := c.repoDetails.ServerAddress
	if addr == "" {
		return errors.NotValidf("empty server address for %q", c.repoDetails.Repository)
	}
	url, err := url.Parse(addr)
	if err != nil {
		return errors.Trace(err)
	}
	serverAddressURL := *url
	apiVersion := c.APIVersion().String()
	if !strings.Contains(url.Path, "/"+apiVersion) {
		url.Path = path.Join(url.Path, apiVersion)
	}
	if url.Scheme == "" {
		url.Scheme = "https"
	}
	c.baseURL = url

	serverAddressURL.Scheme = ""
	c.repoDetails.ServerAddress = serverAddressURL.String()
	logger.Criticalf("baseClient.DecideBaseURL c.baseURL %q, r.repoDetails.ServerAddress %q", c.baseURL, c.repoDetails.ServerAddress)
	return nil
}

func (c baseClient) url(pathTemplate string, args ...interface{}) string {
	pathSuffix := fmt.Sprintf(pathTemplate, args...)
	url := *c.baseURL
	ver := c.APIVersion().String()
	if !strings.HasSuffix(strings.TrimRight(url.Path, "/"), ver) {
		url.Path = path.Join(url.Path, ver)
	}
	if url.Scheme == "" {
		url.Scheme = "https"
	}
	url.Path = path.Join(url.Path, pathSuffix)
	logger.Criticalf("baseClient url.Path ===> %q, %q", url.Path, pathSuffix)
	logger.Criticalf("baseClient c.baseURL ===> %q, url.String() ===> %q", c.baseURL, url.String())
	return url.String()
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
	logger.Criticalf("baseClient.getPaginatedJSON url ===> %q", url)
	resp, err := c.client.Get(url)
	logger.Criticalf("getPaginatedJSON err %#v", err)
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
