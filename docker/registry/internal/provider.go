// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

// NewBase creates a new base provider.
func NewBase(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) *baseClient {
	return newBase(repoDetails, transport, normalizeRepoDetailsCommon)
}

// Providers returns all the supported registry providers.
func Providers() []func(docker.ImageRepoDetails, http.RoundTripper) RegistryInternal {
	return []func(docker.ImageRepoDetails, http.RoundTripper) RegistryInternal{
		newAzureContainerRegistry,
		newGitlabContainerRegistry,
		newGithubContainerRegistry,
		newQuayContainerRegistry,
		newGoogleContainerRegistry,
		newElasticContainerRegistry,
		newElasticContainerRegistryPublic,
		newDockerhub, // DockerHub must be last as it matches on default domain.
	}
}

// InitProvider does some initialization steps for a provider.
func InitProvider(c Initializer) error {
	if err := c.DecideBaseURL(); err != nil {
		return errors.Trace(err)
	}
	if err := c.WrapTransport(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
