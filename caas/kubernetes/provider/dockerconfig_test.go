// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
)

type DockerConfigSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&DockerConfigSuite{})

func (s *DockerConfigSuite) TestExtractRegistryURL(c *gc.C) {
	result := provider.ExtractRegistryURL("test")
	c.Assert(result, gc.Equals, "docker.io")

	result = provider.ExtractRegistryURL("tester/caas-mysql/mysql-image@sha256:dead-beef")
	c.Assert(result, gc.Equals, "docker.io")

	result = provider.ExtractRegistryURL("registry.staging.jujucharms.com/tester/caas-mysql/mysql-image@sha256:dead-beef")
	c.Assert(result, gc.Equals, "registry.staging.jujucharms.com")

}

func (s *DockerConfigSuite) TestCreateDockerConfigJSON(c *gc.C) {
	imageDetails := caas.ImageDetails{
		ImagePath: "registry.staging.jujucharms.com/tester/caas-mysql/mysql-image@sha256:dead-beef",
		Username:  "docker-registry",
		Password:  "hunter2",
	}

	config, err := provider.CreateDockerConfigJSON(&imageDetails)
	c.Assert(err, jc.ErrorIsNil)

	var result provider.DockerConfigJson
	err = json.Unmarshal(config, &result)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, provider.DockerConfigJson{
		Auths: map[string]provider.DockerConfigEntry{
			"registry.staging.jujucharms.com": {
				Username: "docker-registry",
				Password: "hunter2",
				Email:    "",
			},
		},
	})
}
