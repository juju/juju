// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker_test

import (
	"encoding/json"
	"testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	apidocker "github.com/juju/juju/api/docker"
	"github.com/juju/juju/internal/docker"
)

type DockerResourceSuite struct{}

func TestDockerResourceSuite(t *testing.T) {
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

// TestAPIDockerYAMLCompatibility verifies that YAML marshalled by the public
// api/docker type can be unmarshalled by UnmarshalDockerResource. This is the
// core guarantee that allows external clients (e.g. the Terraform provider)
// to use the api/docker types instead of mirroring the internal structs.
// If the struct tags ever diverge, this test fails at the source of truth.
func (s *DockerResourceSuite) TestAPIDockerYAMLCompatibility(c *tc.C) {
	apiDetails := apidocker.DockerImageDetails{
		RegistryPath: "registry.example.com/myimage@sha256:abcdef",
		ImageRepoDetails: apidocker.ImageRepoDetails{
			BasicAuthConfig: apidocker.BasicAuthConfig{
				Username: "user",
				Password: "pass",
			},
			Repository:    "myrepo",
			ServerAddress: "registry.example.com",
		},
	}

	data, err := yaml.Marshal(apiDetails)
	c.Assert(err, tc.ErrorIsNil)

	result, err := docker.UnmarshalDockerResource(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.RegistryPath, tc.Equals, "registry.example.com/myimage@sha256:abcdef")
	c.Assert(result.Username, tc.Equals, "user")
	c.Assert(result.Password, tc.Equals, "pass")
	c.Assert(result.Repository, tc.Equals, "myrepo")
	c.Assert(result.ServerAddress, tc.Equals, "registry.example.com")
}

// TestAPIDockerJSONCompatibility verifies that JSON marshalled by the public
// api/docker type can be unmarshalled by UnmarshalDockerResource.
func (s *DockerResourceSuite) TestAPIDockerJSONCompatibility(c *tc.C) {
	apiDetails := apidocker.DockerImageDetails{
		RegistryPath: "registry.example.com/myimage@sha256:abcdef",
		ImageRepoDetails: apidocker.ImageRepoDetails{
			BasicAuthConfig: apidocker.BasicAuthConfig{
				Username: "user",
				Password: "pass",
			},
			Repository:    "myrepo",
			ServerAddress: "registry.example.com",
		},
	}

	data, err := json.Marshal(apiDetails)
	c.Assert(err, tc.ErrorIsNil)

	result, err := docker.UnmarshalDockerResource(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.RegistryPath, tc.Equals, "registry.example.com/myimage@sha256:abcdef")
	c.Assert(result.Username, tc.Equals, "user")
	c.Assert(result.Password, tc.Equals, "pass")
	c.Assert(result.Repository, tc.Equals, "myrepo")
	c.Assert(result.ServerAddress, tc.Equals, "registry.example.com")
}

// TestInternalDockerYAMLToAPI verifies that YAML produced by the internal
// type can be consumed by the api/docker type. This ensures the controller's
// output is readable by external clients.
func (s *DockerResourceSuite) TestInternalDockerYAMLToAPI(c *tc.C) {
	internalDetails := docker.DockerImageDetails{
		RegistryPath: "registry.example.com/myimage@sha256:abcdef",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "user",
				Password: "pass",
			},
			Repository:    "myrepo",
			ServerAddress: "registry.example.com",
		},
	}

	data, err := yaml.Marshal(internalDetails)
	c.Assert(err, tc.ErrorIsNil)

	var apiDetails apidocker.DockerImageDetails
	err = yaml.Unmarshal(data, &apiDetails)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apiDetails.RegistryPath, tc.Equals, "registry.example.com/myimage@sha256:abcdef")
	c.Assert(apiDetails.Username, tc.Equals, "user")
	c.Assert(apiDetails.Password, tc.Equals, "pass")
	c.Assert(apiDetails.Repository, tc.Equals, "myrepo")
	c.Assert(apiDetails.ServerAddress, tc.Equals, "registry.example.com")
}
