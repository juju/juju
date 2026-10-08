// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"encoding/base64"
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/kubernetes/utils"
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

	expectedAuth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	c.Assert(result, jc.DeepEquals, utils.DockerConfigJSON{
		Auths: map[string]utils.DockerConfigEntry{
			"registry.staging.jujucharms.com": {
				Username: "docker-registry",
				Password: "hunter2",
				Email:    "",
				Auth:     expectedAuth,
			},
		},
	})

	var rawMap map[string]map[string]map[string]string
	err = json.Unmarshal(config, &rawMap)
	c.Assert(err, jc.ErrorIsNil)
	entry := rawMap["auths"]["registry.staging.jujucharms.com"]
	c.Assert(entry, gc.HasLen, 3)
	c.Assert(entry["username"], gc.Equals, "docker-registry")
	c.Assert(entry["password"], gc.Equals, "hunter2")
	c.Assert(entry["auth"], gc.Equals, expectedAuth)
}

func (s *DockerConfigSuite) TestCreateDockerConfigJSONEmptyUsername(c *gc.C) {
	imagePath := "registry.staging.jujucharms.com/tester/caas-mysql/mysql-image:5.7"
	password := "hunter2"

	config, err := utils.CreateDockerConfigJSON("", password, imagePath)
	c.Assert(err, jc.ErrorIsNil)

	expectedAuth := base64.StdEncoding.EncodeToString([]byte(":" + password))
	var rawMap map[string]map[string]map[string]string
	err = json.Unmarshal(config, &rawMap)
	c.Assert(err, jc.ErrorIsNil)
	entry := rawMap["auths"]["registry.staging.jujucharms.com"]
	c.Assert(entry, gc.HasLen, 2)
	c.Assert(entry["password"], gc.Equals, "hunter2")
	c.Assert(entry["auth"], gc.Equals, expectedAuth)
	_, hasUsername := entry["username"]
	c.Assert(hasUsername, jc.IsFalse)
}

func (s *DockerConfigSuite) TestCreateDockerConfigJSONEmptyCredentials(c *gc.C) {
	imagePath := "registry.staging.jujucharms.com/tester/caas-mysql/mysql-image:5.7"

	config, err := utils.CreateDockerConfigJSON("", "", imagePath)
	c.Assert(err, jc.ErrorIsNil)

	var rawMap map[string]map[string]map[string]string
	err = json.Unmarshal(config, &rawMap)
	c.Assert(err, jc.ErrorIsNil)
	entry := rawMap["auths"]["registry.staging.jujucharms.com"]
	c.Assert(entry, gc.HasLen, 0)
}

func (s *DockerConfigSuite) TestCreateDockerConfigJSONExtractRegistryURLError(c *gc.C) {
	imagePath := "me/mygitlab:latest"

	_, err := utils.CreateDockerConfigJSON("docker-registry", "hunter2", imagePath)
	c.Assert(err, gc.ErrorMatches, `oci reference "me/mygitlab:latest" must have a domain`)
}
