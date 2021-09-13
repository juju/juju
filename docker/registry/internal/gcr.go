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

func newGoogleContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &googleContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *googleContainerRegistry) Match() bool {
	c.prepare()
	return strings.Contains(c.repoDetails.ServerAddress, "gcr.io")
}

const (
	googleContainerRegistryUserNameJSONKey = "_json_key"
)

var inValidGoogleContainerRegistryUserNameError = errors.NewNotValid(nil,
	fmt.Sprintf("google container registry username has to be %q",
		googleContainerRegistryUserNameJSONKey,
	),
)

func validateGoogleContainerRegistryCredential(auth docker.BasicAuthConfig) (err error) {
	if auth.Username == "" && auth.Auth == "" {
		return errors.NewNotValid(nil, "username or auth token is required")
	}
	username := auth.Username
	if auth.Auth != "" {
		username, err = getUserNameFromAuth(auth.Auth)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if username != googleContainerRegistryUserNameJSONKey {
		return inValidGoogleContainerRegistryUserNameError
	}
	return nil
}

func (c *googleContainerRegistry) WrapTransport() error {
	transport := c.client.Transport
	if c.repoDetails.IsPrivate() {
		if !c.repoDetails.BasicAuthConfig.Empty() {
			if err := validateGoogleContainerRegistryCredential(c.repoDetails.BasicAuthConfig); err != nil {
				return errors.Trace(err)
			}
			transport = newTokenTransport(
				transport,
				c.repoDetails.Username, c.repoDetails.Password, c.repoDetails.Auth, "", false,
			)
		}
		if !c.repoDetails.TokenAuthConfig.Empty() {
			return errors.New("google container registry only supports username and password or auth token")
		}
	}
	// TODO(ycliuhw): support gcr public registry.
	c.client.Transport = newErrorTransport(transport)
	return nil
}

// Ping pings the github endpoint.
func (c googleContainerRegistry) Ping() error {
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
	return errors.Trace(err)
}
