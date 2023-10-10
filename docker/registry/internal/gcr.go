// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

type googleContainerRegistry struct {
	*baseClient
}

func newGoogleContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) (RegistryInternal, error) {
	c, err := newBase(repoDetails, transport, normalizeRepoDetailsCommon)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &googleContainerRegistry{c}, nil
}

// Match checks if the repository details matches current provider format.
func (c *googleContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "gcr.io")
}

const (
	googleContainerRegistryUserNameJSONKey = "_json_key"
)

var invalidGoogleContainerRegistryUserNameError = errors.NewNotValid(nil,
	fmt.Sprintf("google container registry username has to be %q",
		googleContainerRegistryUserNameJSONKey,
	),
)

func validateGoogleContainerRegistryCredential(auth docker.BasicAuthConfig) (err error) {
	if auth.Username == "" && auth.Auth.Empty() {
		return errors.NewNotValid(nil, "username or auth token is required")
	}
	username := auth.Username
	if !auth.Auth.Empty() {
		username, _, err = unpackAuthToken(auth.Auth.Value)
		if err != nil {
			return errors.Annotate(err, "getting username from the google container registry auth token")
		}
	}
	if username != googleContainerRegistryUserNameJSONKey {
		return invalidGoogleContainerRegistryUserNameError
	}
	return nil
}

func googleContainerRegistryTransport(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) (http.RoundTripper, error) {
	if !repoDetails.TokenAuthConfig.Empty() {
		return nil, errors.NewNotValid(nil, "google container registry only supports username and password or auth token")
	}
	if repoDetails.BasicAuthConfig.Empty() {
		// Anonymous login.
		return newTokenTransport(transport, "", "", "", "", false), nil
	}
	if err := validateGoogleContainerRegistryCredential(repoDetails.BasicAuthConfig); err != nil {
		return nil, errors.Annotatef(err, "validating the google container registry credential")
	}
	return newTokenTransport(
		transport,
		repoDetails.Username, repoDetails.Password, repoDetails.Auth.Content(), "", false,
	), nil
}

func (c *googleContainerRegistry) WrapTransport(...TransportWrapper) (err error) {
	if c.client.Transport, err = mergeTransportWrappers(
		c.client.Transport, c.repoDetails,
		googleContainerRegistryTransport, wrapErrorTransport,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Ping pings the github endpoint.
func (c googleContainerRegistry) Ping() error {
	if !c.repoDetails.IsPrivate() {
		// gcr.io root path requires authentication.
		// So skip ping for public repositories.
		return nil
	}
	url := c.url("/")
	if !strings.HasSuffix(url, "/") {
		// gcr v2 root endpoint requires the trailing slash(otherwise 404 returns).
		url += "/"
	}
	logger.Debugf("gcr ping %q", url)
	resp, err := c.client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	return errors.Trace(unwrapNetError(err))
}
