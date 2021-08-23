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

// NewRegistry creates a new registery.
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
	if r.baseURL, err = url.Parse(repoDetails.ServerAddress); err != nil {
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

func (r registry) url(pathTemplate string, args ...interface{}) string {
	pathSuffix := fmt.Sprintf(pathTemplate, args...)
	url := *r.baseURL
	if url.Scheme == "" {
		url.Scheme = "https"
	}
	url.Path = path.Join(url.Path, pathSuffix)
	return url.String()
}

func (r registry) Ping() error {
	url := r.url("/v2/")
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

type tagsResponse struct {
	Tags []string `json:"tags"`
}

func (r registry) Tags(imageName string) (versions tools.Versions, err error) {
	path := fmt.Sprintf("%s/%s", r.repoDetails.Repository, imageName)
	url := r.url("/v2/%s/tags/list", path)

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

	var response tagsResponse
	for {
		url, err = r.getPaginatedJSON(url, &response)
		switch err {
		case ErrNoMorePages:
			pushVersions(response.Tags)
			return versions, nil
		case nil:
			pushVersions(response.Tags)
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
	ErrNoMorePages = errors.New("no more pages")
)

func getNextLink(resp *http.Response) (string, error) {
	for _, link := range resp.Header[http.CanonicalHeaderKey("Link")] {
		parts := nextLinkRE.FindStringSubmatch(link)
		if parts != nil {
			return parts[1], nil
		}
	}
	return "", ErrNoMorePages
}
