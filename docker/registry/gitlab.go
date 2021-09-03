// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

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

func (c *gitlabContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "registry.gitlab.com")
}

func (c *gitlabContainerRegistry) WrapTransport() error {
	// TODO(ycliuhw): implement gitlab public registry.
	return c.baseClient.WrapTransport()
}
