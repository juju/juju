// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type BootstrapConfigFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&BootstrapConfigFileSuite{})

const testBootstrapConfigYAML = `
controllers:
  local.aws-test:
    config:
      name: admin
      type: ec2
    credential: default
    cloud: aws
    region: us-east-1
    endpoint: https://us-east-1.amazonaws.com
  local.mallards:
    config:
      name: admin
      type: maas
    cloud: maas
    region: 127.0.0.1
`

var testBootstrapConfig = map[string]jujuclient.BootstrapConfig{
	"local.aws-test": {
		Config: map[string]interface{}{
			"type": "ec2",
			"name": "admin",
		},
		Credential:    "default",
		Cloud:         "aws",
		CloudRegion:   "us-east-1",
		CloudEndpoint: "https://us-east-1.amazonaws.com",
	},
	"local.mallards": {
		Config: map[string]interface{}{
			"type": "maas",
			"name": "admin",
		},
		Cloud:       "maas",
		CloudRegion: "127.0.0.1",
	},
}

func (s *BootstrapConfigFileSuite) TestWriteFile(c *gc.C) {
	writeTestBootstrapConfigFile(c)
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("bootstrap-config.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, testBootstrapConfigYAML[1:])
}

func (s *BootstrapConfigFileSuite) TestReadNoFile(c *gc.C) {
	controllers, err := jujuclient.ReadBootstrapConfigFile("nohere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers, gc.IsNil)
}

func (s *BootstrapConfigFileSuite) TestReadEmptyFile(c *gc.C) {
	path := osenv.JujuXDGDataHomePath("bootstrap-config.yaml")
	err := ioutil.WriteFile(path, []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	configs, err := jujuclient.ReadBootstrapConfigFile(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(configs, gc.HasLen, 0)
}

func parseBootstrapConfig(c *gc.C) map[string]jujuclient.BootstrapConfig {
	configs, err := jujuclient.ParseBootstrapConfig([]byte(testBootstrapConfigYAML))
	c.Assert(err, jc.ErrorIsNil)
	return configs
}

func writeTestBootstrapConfigFile(c *gc.C) map[string]jujuclient.BootstrapConfig {
	configs := parseBootstrapConfig(c)
	err := jujuclient.WriteBootstrapConfigFile(configs)
	c.Assert(err, jc.ErrorIsNil)
	return configs
}

func (s *BootstrapConfigFileSuite) TestParseControllerMetadata(c *gc.C) {
	controllers := parseBootstrapConfig(c)
	var names []string
	for name, _ := range controllers {
		names = append(names, name)
	}
	c.Assert(names, jc.SameContents, []string{"local.mallards", "local.aws-test"})
}

func (s *BootstrapConfigFileSuite) TestParseControllerMetadataError(c *gc.C) {
	controllers, err := jujuclient.ParseBootstrapConfig([]byte("fail me now"))
	c.Assert(err, gc.ErrorMatches, "cannot unmarshal bootstrap config: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `fail me...` into jujuclient.bootstrapConfigCollection")
	c.Assert(controllers, gc.IsNil)
}
