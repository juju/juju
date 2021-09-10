// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/http"
	"strings"

	"github.com/juju/juju/docker"
)

type gitlabContainerRegistry struct {
	*baseClient
}

func newGitlabContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &gitlabContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *gitlabContainerRegistry) Match() bool {
	c.prepare()
	return strings.Contains(c.repoDetails.ServerAddress, "registry.gitlab.com")
}

func (c *gitlabContainerRegistry) WrapTransport() error {
	// TODO(ycliuhw): implement gitlab public registry.
	return c.baseClient.WrapTransport()
}
