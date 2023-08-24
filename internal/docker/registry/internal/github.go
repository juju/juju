// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/docker"
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

func githubContainerRegistryTransport(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) (http.RoundTripper, error) {
	if !repoDetails.TokenAuthConfig.Empty() {
		return nil, errors.NewNotValid(nil, "github only supports username and password or auth token")
	}
	if repoDetails.BasicAuthConfig.Empty() {
		// Anonymous login.
		return newTokenTransport(transport, "", "", "", "", false), nil
	}
	password := repoDetails.Password
	if password == "" {
		if repoDetails.Auth.Empty() {
			return nil, errors.NewNotValid(nil, `github container registry requires {"username", "password"} or {"auth"} token`)
		}
		var err error
		_, password, err = unpackAuthToken(repoDetails.Auth.Value)
		if err != nil {
			return nil, errors.Annotate(err, "getting password from the github container registry auth token")
		}
		if password == "" {
			return nil, errors.NewNotValid(nil, "github container registry auth token contains empty password")
		}
	}
	bearerToken := base64.StdEncoding.EncodeToString([]byte(password))
	return newTokenTransport(transport, "", "", "", bearerToken, true), nil
}

func (c *githubContainerRegistry) WrapTransport(...TransportWrapper) (err error) {
	if c.client.Transport, err = mergeTransportWrappers(
		c.client.Transport, c.repoDetails,
		githubContainerRegistryTransport, wrapErrorTransport,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Ping pings the github endpoint.
func (c githubContainerRegistry) Ping() error {
	if !c.repoDetails.IsPrivate() {
		// V2 API does not support - ping without credential.
		return nil
	}
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
	return unwrapNetError(err)
}
