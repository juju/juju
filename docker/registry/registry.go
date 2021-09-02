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
	"github.com/juju/version/v2"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.docker.registry")

const (
	defaultTimeout = 15 * time.Second
)

var (
	// Override for testing.
	DefaultTransport = http.DefaultTransport
)

func providers() []func(docker.ImageRepoDetails, http.RoundTripper) RegistryInternal {
	return []func(docker.ImageRepoDetails, http.RoundTripper) RegistryInternal{
		newACR,
		newDockerhub,
		newGitlab,
		newGithub,
		newQuay,
		newGCR,
	}
}

func New(repoDetails docker.ImageRepoDetails) (Registry, error) {
	var provider RegistryInternal = newBase(repoDetails, DefaultTransport)
	for _, providerNewer := range providers() {
		p := providerNewer(repoDetails, DefaultTransport)
		if p.Match() {
			provider = p
			break
		}
	}
	if err := initClient(provider); err != nil {
		return nil, errors.Trace(err)
	}
	return provider, nil
}

type baseClient struct {
	baseURL     *url.URL
	client      *http.Client
	repoDetails *docker.ImageRepoDetails
}

func initClient(c Initializer) error {
	if err := c.DecideBaseURL(); err != nil {
		return errors.Trace(err)
	}
	if err := c.WrapTransport(); err != nil {
		return errors.Trace(err)
	}
	if err := c.Ping(); err != nil {
		return errors.Trace(err)
	}
	return nil
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

func (c *baseClient) WrapTransport() error {
	logger.Criticalf("baseClient.WrapTransport")
	if !c.repoDetails.IsPrivate() {
		return nil
	}
	transport := c.client.Transport
	if !c.repoDetails.BasicAuthConfig.Empty() {
		transport = newBasicTransport(
			transport, c.repoDetails.Username, c.repoDetails.Password, c.repoDetails.Auth,
		)
	}
	c.client.Transport = errorTransport{transport}
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
	apiVersion := c.repoDetails.APIVersion().String()
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
	ver := c.repoDetails.APIVersion().String()
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

type tagsResponseLayerV1 struct {
	Name string `json:"name"`
}

type tagsResponseV1 []tagsResponseLayerV1

func (r tagsResponseV1) GetTags() []string {
	var tags []string
	for _, v := range r {
		tags = append(tags, v.Name)
	}
	return tags
}

type tagsResponseV2 struct {
	Tags []string `json:"tags"`
}

func (r tagsResponseV2) GetTags() []string {
	return r.Tags
}

type tagsGetter interface {
	GetTags() []string
}

func getRepositoryOnly(s string) string {
	i := strings.IndexRune(s, '/')
	if i == -1 {
		return s
	}
	return s[i+1:]
}

// Tags fetches tags for an OCI image.
func (c baseClient) Tags(imageName string) (versions tools.Versions, err error) {
	apiVersion := c.repoDetails.APIVersion()

	repo := getRepositoryOnly(c.repoDetails.Repository)
	if apiVersion == docker.APIVersionV1 {
		url := c.url("/repositories/%s/%s/tags", repo, imageName)
		var response tagsResponseV1
		return c.fetchTags(url, &response)
	}
	if apiVersion == docker.APIVersionV2 {
		url := c.url("/%s/%s/tags/list", repo, imageName)
		var response tagsResponseV2
		return c.fetchTags(url, &response)
	}
	// This should never happen.
	return nil, nil
}

func (c baseClient) fetchTags(url string, res tagsGetter) (versions tools.Versions, err error) {
	pushVersions := func(tags []string) {
		for _, tag := range tags {
			v, err := version.Parse(tag)
			if err != nil {
				logger.Warningf("ignoring unexpected image tag %q", tag)
				continue
			}
			versions = append(versions, docker.NewImageInfo(v))
		}
	}
	for {
		url, err = c.getPaginatedJSON(url, &res)
		switch err {
		case errNoMorePages:
			pushVersions(res.GetTags())
			return versions, nil
		case nil:
			pushVersions(res.GetTags())
			continue
		default:
			return nil, errors.Trace(err)
		}
	}
}

func (c baseClient) getPaginatedJSON(url string, response interface{}) (string, error) {
	logger.Criticalf("baseClient.getPaginatedJSON url ===> %q", url)
	resp, err := c.client.Get(url)
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
