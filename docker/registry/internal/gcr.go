// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

type googleContainerRegistry struct {
	*baseClient
}

func newGoogleContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport)
	return &googleContainerRegistry{c}
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
	if auth.Username == "" && auth.Auth == "" {
		return errors.NewNotValid(nil, "username or auth token is required")
	}
	username := auth.Username
	if auth.Auth != "" {
		username, err = getUserNameFromAuth(auth.Auth)
		if err != nil {
			return errors.Annotate(err, "getting username from the google container registry auth token")
		}
	}
	if username != googleContainerRegistryUserNameJSONKey {
		return invalidGoogleContainerRegistryUserNameError
	}
	return nil
}

// APIVersion returns the registry API version to use.
func (c *googleContainerRegistry) APIVersion() APIVersion {
	// google container registry always uses v2.
	return APIVersionV2
}

func (c *googleContainerRegistry) WrapTransport() error {
	transport := c.client.Transport
	if c.repoDetails.IsPrivate() {
		if !c.repoDetails.BasicAuthConfig.Empty() {
			if err := validateGoogleContainerRegistryCredential(c.repoDetails.BasicAuthConfig); err != nil {
				return errors.Annotatef(err, "validating the google container registry credential")
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
	c.client.Transport = newErrorTransport(transport)
	return nil
}

func (c googleContainerRegistry) url(pathTemplate string, args ...interface{}) string {
	return commonURL(c.APIVersion(), *c.baseURL, pathTemplate, args...)
}

// DecideBaseURL decides the API url to use.
func (c *googleContainerRegistry) DecideBaseURL() error {
	return errors.Trace(decideBaseURLCommon(c.APIVersion(), c.repoDetails, c.baseURL))
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
	return errors.Trace(err)
}

// Tags fetches tags for an OCI image.
func (c googleContainerRegistry) Tags(imageName string) (versions tools.Versions, err error) {
	// google container registry always uses v2.
	repo := getRepositoryOnly(c.repoDetails.Repository)
	url := c.url("/%s/%s/tags/list", repo, imageName)
	var response tagsResponseV2
	return c.fetchTags(url, &response)
}
