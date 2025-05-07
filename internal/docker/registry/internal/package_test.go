// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/docker"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type (
	AzureContainerRegistry   = azureContainerRegistry
	BaseClient               = baseClient
	Dockerhub                = dockerhub
	GoogleContainerRegistry  = googleContainerRegistry
	GithubContainerRegistry  = githubContainerRegistry
	GitlabContainerRegistry  = gitlabContainerRegistry
	QuayContainerRegistry    = quayContainerRegistry
	ElasticContainerRegistry = elasticContainerRegistry
)

var (
	NewErrorTransport                  = newErrorTransport
	NewChallengeTransport              = newChallengeTransport
	NewBasicTransport                  = newBasicTransport
	NewTokenTransport                  = newTokenTransport
	NewElasticContainerRegistryForTest = newElasticContainerRegistryForTest
	NewAzureContainerRegistry          = newAzureContainerRegistry
	GetArchitectures                   = getArchitectures
	UnwrapNetError                     = unwrapNetError
)

func (c *BaseClient) SetImageRepoDetails(i docker.ImageRepoDetails) {
	c.repoDetails = &i
}
