// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/testing"
)

type DockerConfigSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&DockerConfigSuite{})

func (s *DockerConfigSuite) TestExtractRegistryURL(c *gc.C) {
	for _, registryTest := range []struct {
		registryPath string
		expectedURL  string
		err          string
	}{{
		registryPath: "registry.staging.charmstore.com/me/awesomeimage@sha256:5e2c71d050bec85c258a31aa4507ca8adb3b2f5158a4dc919a39118b8879a5ce",
		expectedURL:  "registry.staging.charmstore.com",
	}, {
		registryPath: "gcr.io/kubeflow/jupyterhub-k8s@sha256:5e2c71d050bec85c258a31aa4507ca8adb3b2f5158a4dc919a39118b8879a5ce",
		expectedURL:  "gcr.io",
	}, {
		registryPath: "docker.io/me/mygitlab:latest",
		expectedURL:  "docker.io",
	}, {
		registryPath: "me/mygitlab:latest",
		expectedURL:  "",
		err:          `oci reference "me/mygitlab:latest" must have a domain`,
	}} {
		result, err := utils.ExtractRegistryURL(registryTest.registryPath)
		if registryTest.err != "" {
			c.Assert(err, gc.ErrorMatches, registryTest.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
		c.Assert(result, gc.Equals, registryTest.expectedURL)
	}
}

func (s *DockerConfigSuite) TestCreateDockerConfigJSON(c *gc.C) {
	imagePath := "registry.staging.jujucharms.com/tester/caas-mysql/mysql-image:5.7"
	username := "docker-registry"
	password := "hunter2"

	config, err := utils.CreateDockerConfigJSON(username, password, imagePath)
	c.Assert(err, jc.ErrorIsNil)

	var result utils.DockerConfigJSON
	err = json.Unmarshal(config, &result)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, utils.DockerConfigJSON{
		Auths: map[string]utils.DockerConfigEntry{
			"registry.staging.jujucharms.com": {
				Username: "docker-registry",
				Password: "hunter2",
				Email:    "",
			},
		},
	})
}
