// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

func providers() []func(docker.ImageRepoDetails, http.RoundTripper) RegistryInternal {
	return []func(docker.ImageRepoDetails, http.RoundTripper) RegistryInternal{
		newAzureContainerRegistry,
		newDockerhub,
		newGitlabContainerRegistry,
		newGithubContainerRegistry,
		newQuayContainerRegistry,
		newGoogleContainerRegistry,
	}
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

// New returns a Registry interface providing methods for interacting with registry APIs.
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
