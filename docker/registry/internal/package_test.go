// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"testing"

	gc "gopkg.in/check.v1"
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
	NewErrorTransport       = newErrorTransport
	NewBasicTransport       = newBasicTransport
	NewTokenTransport       = newTokenTransport
	NewPrivateOnlyTransport = newPrivateOnlyTransport
)
