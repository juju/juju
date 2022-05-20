// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/docker"
)

type DockerResourceSuite struct{}

var _ = gc.Suite(&DockerResourceSuite{})

func (s *DockerResourceSuite) TestValidRegistryPath(c *gc.C) {
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
		err := resource.ValidateDockerRegistryPath(registryTest.registryPath)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *DockerResourceSuite) TestInvalidRegistryPath(c *gc.C) {
	err := resource.ValidateDockerRegistryPath("blah:sha256@")
	c.Assert(err, gc.ErrorMatches, "docker image path .* not valid")
}

func (s *DockerResourceSuite) TestDockerImageDetailsUnmarshalJson(c *gc.C) {
	data := []byte(`{"ImageName":"testing@sha256:beef-deed","Username":"docker-registry","Password":"fragglerock"}`)
	result, err := resource.UnmarshalDockerResource(data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, resource.DockerImageDetails{
		RegistryPath: "testing@sha256:beef-deed",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "docker-registry",
				Password: "fragglerock",
			},
		},
	})
}

func (s *DockerResourceSuite) TestDockerImageDetailsUnmarshalYaml(c *gc.C) {
	data := []byte(`
registrypath: testing@sha256:beef-deed
username: docker-registry
password: fragglerock
`[1:])
	result, err := resource.UnmarshalDockerResource(data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, resource.DockerImageDetails{
		RegistryPath: "testing@sha256:beef-deed",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "docker-registry",
				Password: "fragglerock",
			},
		},
	})
}
