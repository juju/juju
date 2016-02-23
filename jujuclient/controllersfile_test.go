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

type ControllersFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ControllersFileSuite{})

const testControllersYAML = `
controllers:
  local.aws-test:
    servers: [instance-1-2-4.useast.aws.com]
    uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
  local.mallards:
    servers: [maas-1-05.cluster.mallards]
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
  local.mark-test-prodstack:
    servers: [vm-23532.prodstack.canonical.com, great.test.server.hostname.co.nz]
    uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    ca-cert: this-is-a-ca-cert
`

func (s *ControllersFileSuite) TestWriteFile(c *gc.C) {
	writeTestControllersFile(c)
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("controllers.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, testControllersYAML[1:])
}

func (s *ControllersFileSuite) TestReadNoFile(c *gc.C) {
	controllers, err := jujuclient.ReadControllersFile("nohere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers, gc.IsNil)
}

func (s *ControllersFileSuite) TestReadEmptyFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("controllers.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	controllerStore := jujuclient.NewFileClientStore()
	controllers, err := controllerStore.AllControllers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers, gc.IsNil)
}

func parseControllers(c *gc.C) map[string]jujuclient.ControllerDetails {
	controllers, err := jujuclient.ParseControllers([]byte(testControllersYAML))
	c.Assert(err, jc.ErrorIsNil)

	// ensure that multiple server hostnames and eapi endpoints are parsed correctly
	c.Assert(controllers["local.mark-test-prodstack"].Servers, gc.HasLen, 2)
	c.Assert(controllers["local.mallards"].APIEndpoints, gc.HasLen, 2)
	return controllers
}

func writeTestControllersFile(c *gc.C) map[string]jujuclient.ControllerDetails {
	controllers := parseControllers(c)
	err := jujuclient.WriteControllersFile(controllers)
	c.Assert(err, jc.ErrorIsNil)
	return controllers
}

func (s *ControllersFileSuite) TestParseControllerMetadata(c *gc.C) {
	controllers := parseControllers(c)
	var names []string
	for name, _ := range controllers {
		names = append(names, name)
	}
	c.Assert(names, jc.SameContents,
		[]string{"local.mark-test-prodstack", "local.mallards", "local.aws-test"})
}

func (s *ControllersFileSuite) TestParseControllerMetadataError(c *gc.C) {
	controllers, err := jujuclient.ParseControllers([]byte("fail me now"))
	c.Assert(err, gc.ErrorMatches, "cannot unmarshal yaml controllers metadata: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `fail me...` into jujuclient.controllersCollection")
	c.Assert(controllers, gc.IsNil)
}
