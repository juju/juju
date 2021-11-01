// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker"
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
)

func (c *BaseClient) SetImageRepoDetails(i docker.ImageRepoDetails) {
	c.repoDetails = &i
}

// func (c baseClient) GetBlobs(imageName, digest string) (*BlobsResponse, error) {
// 	return c.GetBlobs(imageName, digest)
// }

// func (c baseClient) GetManifests(imageName, tag string) (*ManifestsResult, error) {
// 	return c.GetManifests(imageName, tag)
// }

// func (c azureContainerRegistry) GetBlobs(imageName, digest string) (*BlobsResponse, error) {
// 	return c.GetBlobs(imageName, digest)
// }

// func (c azureContainerRegistry) GetManifests(imageName, tag string) (*ManifestsResult, error) {
// 	return c.GetManifests(imageName, tag)
// }

// func (c ElasticContainerRegistry) GetBlobs(imageName, digest string) (*BlobsResponse, error) {
// 	return c.GetBlobs(imageName, digest)
// }

// func (c ElasticContainerRegistry) GetManifests(imageName, tag string) (*ManifestsResult, error) {
// 	return c.GetManifests(imageName, tag)
// }
