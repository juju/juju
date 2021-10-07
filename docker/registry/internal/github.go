// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

type githubContainerRegistry struct {
	*baseClient
}

func newGithubContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport, normalizeRepoDetailsCommon)
	return &githubContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *githubContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "ghcr.io")
}

func getBearerTokenForGithub(auth string) (string, error) {
	if auth == "" {
		return "", errors.NotValidf("empty github container registry auth token")
	}
	content, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", errors.Trace(err)
	}
	parts := strings.Split(string(content), ":")
	if len(parts) < 2 {
		return "", errors.NotValidf("github container registry auth token %q", auth)
	}
	token := parts[1]
	return base64.StdEncoding.EncodeToString([]byte(token)), nil
}

func githubContainerRegistryTransport(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) (http.RoundTripper, error) {
	if !repoDetails.BasicAuthConfig.Empty() {
		bearerToken, err := getBearerTokenForGithub(repoDetails.Auth.Value)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newTokenTransport(
			transport, "", "", "", bearerToken, true,
		), nil
	}
	if !repoDetails.TokenAuthConfig.Empty() {
		return nil, errors.NewNotValid(nil, "github only supports username and password or auth token")
	}
	return transport, nil
}

func (c *githubContainerRegistry) WrapTransport(...TransportWrapper) (err error) {
	if c.client.Transport, err = mergeTransportWrappers(
		c.client.Transport, c.repoDetails,
		newPrivateOnlyTransport, githubContainerRegistryTransport, wrapErrorTransport,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Ping pings the github endpoint.
func (c githubContainerRegistry) Ping() error {
	url := c.url("/")
	if !strings.HasSuffix(url, "/") {
		// github v2 root endpoint requires the trailing slash(otherwise 404 returns).
		url += "/"
	}
	logger.Debugf("github ping %q", url)
	resp, err := c.client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	return err
}
