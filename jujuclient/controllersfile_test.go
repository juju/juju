// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
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
    aws-test:
        uuid: this-is-the-aws-test-uuid
        api-endpoints: [this-is-aws-test-of-many-api-endpoints]
        dns-cache: {example.com: [0.1.1.1, 0.2.2.2]}
        ca-cert: this-is-aws-test-ca-cert
        cloud: aws
        region: us-east-1
        controller-machine-count: 0
        active-controller-machine-count: 0
    mallards:
        uuid: this-is-another-uuid
        api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
        ca-cert: this-is-another-ca-cert
        cloud: mallards
        controller-machine-count: 0
        active-controller-machine-count: 0
    mark-test-prodstack:
        uuid: this-is-a-uuid
        api-endpoints: [this-is-one-of-many-api-endpoints]
        ca-cert: this-is-a-ca-cert
        cloud: prodstack
        controller-machine-count: 0
        active-controller-machine-count: 0
current-controller: mallards
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
	c.Assert(controllers, gc.NotNil)
	c.Assert(controllers.Controllers, gc.HasLen, 0)
	c.Assert(controllers.CurrentController, gc.Equals, "")
}

func (s *ControllersFileSuite) TestReadPermissionsError(c *gc.C) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file-%d", time.Now().UnixNano()))
	err := ioutil.WriteFile(path, []byte(""), 0377)
	c.Assert(err, jc.ErrorIsNil)

	_, err = jujuclient.ReadControllersFile(path)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "open .*: permission denied")
}

func (s *ControllersFileSuite) TestReadEmptyFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("controllers.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	controllerStore := jujuclient.NewFileClientStore()
	controllers, err := controllerStore.AllControllers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers, gc.IsNil)
}

func parseControllers(c *gc.C) *jujuclient.Controllers {
	controllers, err := jujuclient.ParseControllers([]byte(testControllersYAML))
	c.Assert(err, jc.ErrorIsNil)

	// ensure that multiple server hostnames and eapi endpoints are parsed correctly
	c.Assert(controllers.Controllers["mallards"].APIEndpoints, gc.HasLen, 2)
	return controllers
}

func writeTestControllersFile(c *gc.C) *jujuclient.Controllers {
	controllers := parseControllers(c)
	err := jujuclient.WriteControllersFile(controllers)
	c.Assert(err, jc.ErrorIsNil)
	return controllers
}

func (s *ControllersFileSuite) TestParseControllerMetadata(c *gc.C) {
	controllers := parseControllers(c)
	var names []string
	for name := range controllers.Controllers {
		names = append(names, name)
	}
	c.Assert(names, jc.SameContents,
		[]string{"mark-test-prodstack", "mallards", "aws-test"},
	)
	c.Assert(controllers.CurrentController, gc.Equals, "mallards")
}

func (s *ControllersFileSuite) TestParseControllerMetadataError(c *gc.C) {
	controllers, err := jujuclient.ParseControllers([]byte("fail me now"))
	c.Assert(err, gc.ErrorMatches, "cannot unmarshal yaml controllers metadata: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `fail me...` into jujuclient.Controllers")
	c.Assert(controllers, gc.IsNil)
}

func (s *ControllersFileSuite) TestControllerFileOldFormat(c *gc.C) {
	fileContent := `
controllers:
    aws-test:
        uuid: this-is-the-aws-test-uuid
        api-endpoints: [this-is-aws-test-of-many-api-endpoints]
        dns-cache: {example.com: [0.1.1.1, 0.2.2.2]}
        ca-cert: this-is-aws-test-ca-cert
        cloud: aws
        region: us-east-1
        controller-machine-count: 0
        active-controller-machine-count: 0%s
current-controller: aws-test
`
	modelCount := `
        model-count: 2`
	fileName := "controllers.yaml"

	// Contains model-count.
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath(fileName), []byte(fmt.Sprintf(fileContent, modelCount)[1:]), 0600)
	c.Assert(err, jc.ErrorIsNil)

	controllerStore := jujuclient.NewFileClientStore()
	controllers, err := controllerStore.AllControllers()
	c.Assert(err, jc.ErrorIsNil)

	expectedDetails := jujuclient.ControllerDetails{
		ControllerUUID: "this-is-the-aws-test-uuid",
		APIEndpoints:   []string{"this-is-aws-test-of-many-api-endpoints"},
		DNSCache:       map[string][]string{"example.com": {"0.1.1.1", "0.2.2.2"}},
		CACert:         "this-is-aws-test-ca-cert",
		Cloud:          "aws",
		CloudRegion:    "us-east-1",
	}
	c.Assert(controllers, gc.DeepEquals, map[string]jujuclient.ControllerDetails{
		"aws-test": expectedDetails,
	})

	err = controllerStore.UpdateController("aws-test", expectedDetails)
	c.Assert(err, jc.ErrorIsNil)

	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath(fileName))
	c.Assert(err, jc.ErrorIsNil)

	// Has no model-count reference.
	c.Assert(string(data), gc.Equals, fmt.Sprintf(fileContent, "")[1:])
}
