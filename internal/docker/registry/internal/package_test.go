// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/docker"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type (
	AzureContainerRegistry         = azureContainerRegistry
	BaseClient                     = baseClient
	Dockerhub                      = dockerhub
	GoogleContainerRegistry        = googleContainerRegistry
	GithubContainerRegistry        = githubContainerRegistry
	GitlabContainerRegistry        = gitlabContainerRegistry
	QuayContainerRegistry          = quayContainerRegistry
	ElasticContainerRegistry       = elasticContainerRegistry
	ElasticContainerRegistryPublic = elasticContainerRegistryPublic
)

var (
	NewErrorTransport                  = newErrorTransport
	NewBasicTransport                  = newBasicTransport
	NewTokenTransport                  = newTokenTransport
	NewElasticContainerRegistryForTest = newElasticContainerRegistryForTest
	NewAzureContainerRegistry          = newAzureContainerRegistry
	GetArchitecture                    = getArchitecture
	UnwrapNetError                     = unwrapNetError
)

func (c *BaseClient) SetImageRepoDetails(i docker.ImageRepoDetails) {
	c.repoDetails = &i
}
