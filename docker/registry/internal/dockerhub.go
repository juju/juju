// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

const (
	dockerServerAddress = "index.docker.io"
)

type dockerhub struct {
	*baseClient
}

func newDockerhub(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport, normalizeRepoDetailsCommon)
	return &dockerhub{c}
}

// Match checks if the repository details matches current provider format.
func (c *dockerhub) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "docker.io")
}

// DecideBaseURL decides the API url to use.
func (c *dockerhub) DecideBaseURL() error {
	c.repoDetails.ServerAddress = dockerServerAddress
	if err := c.baseClient.DecideBaseURL(); err != nil {
		return errors.Trace(err)
	}
	url, err := url.Parse(c.repoDetails.ServerAddress)
	if err != nil {
		return errors.Trace(err)
	}
	url.Scheme = "https"
	addr := url.String()
	if !strings.HasSuffix(addr, "/") {
		// This "/" matters because docker uses url string for the credential key and expects the trailing slash.
		addr += "/"
	}
	c.repoDetails.ServerAddress = addr
	logger.Tracef("dockerhub repoDetails %s", c.repoDetails)
	return nil
}
