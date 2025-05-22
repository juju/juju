// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/docker"
)

type DockerResourceSuite struct{}

func TestDockerResourceSuite(t *stdtesting.T) {
	tc.Run(t, &DockerResourceSuite{})
}

func (s *DockerResourceSuite) TestValidRegistryPath(c *tc.C) {
	for _, registryTest := range []struct {
		registryPath string
	}{{
		registryPath: "registry.staging.charmstore.com/me/awesomeimage@sha256:5e2c71d050bec85c258a31aa4507ca8adb3b2f5158a4dc919a39118b8879a5ce",
	}, {
		registryPath: "gcr.io/kubeflow/jupyterhub-k8s@sha256:5e2c71d050bec85c258a31aa4507ca8adb3b2f5158a4dc919a39118b8879a5ce",
	}, {
		registryPath: "docker.io/me/mygitlab:latest",
	}, {
		registryPath: "me/mygitlab:latest",
	}} {
		err := docker.ValidateDockerRegistryPath(registryTest.registryPath)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *DockerResourceSuite) TestInvalidRegistryPath(c *tc.C) {
	err := docker.ValidateDockerRegistryPath("blah:sha256@")
	c.Assert(err, tc.ErrorMatches, "docker image path .* not valid")
}

func (s *DockerResourceSuite) TestDockerImageDetailsUnmarshalJson(c *tc.C) {
	data := []byte(`{"ImageName":"testing@sha256:beef-deed","Username":"docker-registry","Password":"fragglerock"}`)
	result, err := docker.UnmarshalDockerResource(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, docker.DockerImageDetails{
		RegistryPath: "testing@sha256:beef-deed",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "docker-registry",
				Password: "fragglerock",
			},
		},
	})
}

func (s *DockerResourceSuite) TestDockerImageDetailsUnmarshalYaml(c *tc.C) {
	data := []byte(`
registrypath: testing@sha256:beef-deed
username: docker-registry
password: fragglerock
`[1:])
	result, err := docker.UnmarshalDockerResource(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, docker.DockerImageDetails{
		RegistryPath: "testing@sha256:beef-deed",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "docker-registry",
				Password: "fragglerock",
			},
		},
	})
}
