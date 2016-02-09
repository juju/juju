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

type FileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&FileSuite{})

func (s *FileSuite) TestWriteFile(c *gc.C) {
	writeTestControllersFile(c)
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("controllers.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, testControllersYAML[1:])
}

func (s *FileSuite) TestReadNoFile(c *gc.C) {
	controllers, err := jujuclient.ReadControllersFile("nohere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers, gc.IsNil)
}

func (s *FileSuite) TestReadEmptyFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("controllers.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	controllerStore, err := jujuclient.DefaultControllerStore()
	c.Assert(err, jc.ErrorIsNil)
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

func (s *FileSuite) TestParseControllerMetadata(c *gc.C) {
	controllers := parseControllers(c)
	var names []string
	for name, _ := range controllers {
		names = append(names, name)
	}
	c.Assert(names, jc.SameContents,
		[]string{"local.mark-test-prodstack", "local.mallards", "local.aws-test"})
}

func (s *FileSuite) TestParseControllerMetadataError(c *gc.C) {
	controllers, err := jujuclient.ParseControllers([]byte("fail me now"))
	c.Assert(err, gc.ErrorMatches, "cannot unmarshal yaml controllers metadata: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `fail me...` into jujuclient.controllerDetailsList")
	c.Assert(controllers, gc.IsNil)
}
