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

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/http_mock.go net/http RoundTripper
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/registry_mock.go github.com/juju/juju/docker/registry Registry

const (
	dockerServerAddressV1 = "https://registry.hub.docker.com/"
	dockerServerAddressV2 = "https://index.docker.io/"
)

type registry struct {
	baseURL     *url.URL
	client      *http.Client
	repoDetails *docker.ImageRepoDetails
}

// Registry provides APIs to interact with the registry.
type Registry interface {
	Tags(string) (tools.Versions, error)
	Close() error
	Ping() error
}

// NewRegistry creates a new registry.
func NewRegistry(repoDetails docker.ImageRepoDetails) (Registry, error) {
	return newRegistry(repoDetails, http.DefaultTransport)
}

func newRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) (Registry, error) {
	r := &registry{
		repoDetails: &repoDetails,
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
	}
	var err error
	if r.baseURL, err = r.decideBaseURL(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := newClientWithOpts(r, wrapTransport); err != nil {
		return nil, errors.Trace(err)
	}
	if err = r.Ping(); err != nil {
		return nil, errors.Trace(err)
	}
	return r, nil
}

type opt func(*registry) error

func newClientWithOpts(r *registry, ops ...opt) error {
	for _, op := range ops {
		if err := op(r); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (r registry) decideBaseURL() (*url.URL, error) {
	if r.repoDetails.ServerAddress != "" {
		return url.Parse(r.repoDetails.ServerAddress)
	}
	switch r.repoDetails.APIVersion() {
	case docker.APIVersionV1:
		return url.Parse(dockerServerAddressV1)
	case docker.APIVersionV2:
		return url.Parse(dockerServerAddressV2)
	default:
		// This should never happen.
		return nil, errors.NewNotValid(nil, "cant not decide base url for image repo details")
	}
}

func (r registry) url(pathTemplate string, args ...interface{}) string {
	pathSuffix := fmt.Sprintf(pathTemplate, args...)
	url := *r.baseURL
	if url.Scheme == "" {
		url.Scheme = "https"
	}
	url.Path = path.Join(url.Path, pathSuffix)
	return url.String()
}

// Ping pings the base endpoint.
func (r registry) Ping() error {
	url := r.url("/")
	logger.Debugf("registry ping %q", url)
	resp, err := r.client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	return errors.Trace(err)
}

// Close closes the transport used by the client.
func (r *registry) Close() error {
	if t, ok := r.client.Transport.(*http.Transport); ok {
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

// Tags fetches tags for an OCI image.
func (r registry) Tags(imageName string) (versions tools.Versions, err error) {
	// TODO: merge ListOperatorImages with registry and refactor registry to embed different API version handllers properly.
	apiVersion := r.repoDetails.APIVersion()

	if r.repoDetails.APIVersion() == docker.APIVersionV1 {
		urlTemplate := "/%s/repositories/%s/%s/tags"
		url := r.url(urlTemplate, string(apiVersion), r.repoDetails.Repository, imageName)
		var response tagsResponseV1
		return r.fetchTags(url, &response)
	}
	if r.repoDetails.APIVersion() == docker.APIVersionV2 {
		urlTemplate := "/%s/%s/%s/tags/list"
		url := r.url(urlTemplate, string(apiVersion), r.repoDetails.Repository, imageName)
		var response tagsResponseV2
		return r.fetchTags(url, &response)
	}

	return nil, nil
}

func (r registry) fetchTags(url string, res tagsGetter) (versions tools.Versions, err error) {
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
		url, err = r.getPaginatedJSON(url, &res)
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

func (r registry) getPaginatedJSON(url string, response interface{}) (string, error) {
	resp, err := r.client.Get(url)
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
