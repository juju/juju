// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/docker"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.docker.registry.internal")

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

func newBase(
	repoDetails docker.ImageRepoDetails, transport http.RoundTripper,
	normalizeRepoDetails func(repoDetails *docker.ImageRepoDetails) error,
) (*baseClient, error) {
	c := &baseClient{
		baseURL:     &url.URL{},
		repoDetails: &repoDetails,
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
	}
	err := normalizeRepoDetails(c.repoDetails)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c, nil
}

// normalizeRepoDetailsCommon pre-processes ImageRepoDetails before Match().
func normalizeRepoDetailsCommon(repoDetails *docker.ImageRepoDetails) error {
	if repoDetails.ServerAddress != "" {
		return nil
	}
	// We have validated the repository in top level.
	// It should not raise errors here.
	named, _ := reference.ParseNormalizedNamed(repoDetails.Repository)
	domain := reference.Domain(named)
	if domain == "docker.io" && !strings.HasPrefix(strings.ToLower(repoDetails.Repository), "docker.io") {
		return fmt.Errorf("oci reference %q must have a domain", repoDetails.Repository)
	}
	if domain != "" {
		repoDetails.ServerAddress = domain
	}
	return nil
}

func (c *baseClient) String() string {
	return "generic"
}

// ShouldRefreshAuth checks if the repoDetails should be refreshed.
func (c *baseClient) ShouldRefreshAuth() (bool, time.Duration) {
	return false, time.Duration(0)
}

// RefreshAuth refreshes the repoDetails.
func (c *baseClient) RefreshAuth() error {
	return nil
}

// Match checks if the repository details matches current provider format.
func (c *baseClient) Match() bool {
	return false
}

// APIVersion returns the registry API version to use.
func (c *baseClient) APIVersion() APIVersion {
	return APIVersionV2
}

// TransportWrapper wraps RoundTripper.
type TransportWrapper func(http.RoundTripper, *docker.ImageRepoDetails) (http.RoundTripper, error)

func transportCommon(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails) (http.RoundTripper, error) {
	if !repoDetails.TokenAuthConfig.Empty() {
		return nil, errors.NewNotValid(nil,
			fmt.Sprintf(
				`only {"username", "password"} or {"auth"} authorization is supported for registry %q`,
				repoDetails.ServerAddress,
			),
		)
	}
	return newChallengeTransport(
		transport, repoDetails.Username, repoDetails.Password, repoDetails.Auth.Content(),
	), nil
}

func mergeTransportWrappers(
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
	if c.client.Transport, err = mergeTransportWrappers(c.client.Transport, c.repoDetails, wrappers...); err != nil {
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
	logger.Tracef(context.TODO(), "baseClient repoDetails %s", repoDetails)
	return nil
}

// DecideBaseURL decides the API url to use.
func (c *baseClient) DecideBaseURL() error {
	return errors.Trace(decideBaseURLCommon(c.APIVersion(), c.repoDetails, c.baseURL))
}

func commonURLGetter(version APIVersion, url url.URL, pathTemplate string, args ...interface{}) string {
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

func (c *baseClient) url(pathTemplate string, args ...interface{}) string {
	return commonURLGetter(c.APIVersion(), *c.baseURL, pathTemplate, args...)
}

// Ping pings the baseClient endpoint.
func (c *baseClient) Ping() error {
	url := c.url("/")
	logger.Debugf(context.TODO(), "baseClient ping %q", url)
	resp, err := c.client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	return errors.Trace(unwrapNetError(err))
}

func (c *baseClient) ImageRepoDetails() (o docker.ImageRepoDetails) {
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

func (c *baseClient) getPaginatedJSON(url string, response interface{}) (string, error) {
	resp, err := c.client.Get(url)
	logger.Tracef(context.TODO(), "getPaginatedJSON for %q, err %v", url, err)
	if err != nil {
		return "", errors.Trace(unwrapNetError(err))
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

// unpackAuthToken returns the unpacked username and password.
func unpackAuthToken(auth string) (username string, password string, err error) {
	content, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", "", errors.Annotate(err, "doing base64 decode on the auth token")
	}
	parts := strings.Split(string(content), ":")
	if len(parts) < 2 {
		return "", "", errors.NotValidf("registry auth token")
	}
	return parts[0], parts[1], nil
}
