// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type ControllersFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&ControllersFileSuite{})

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
has-controller-changed-on-previous-switch: false
`

func (s *ControllersFileSuite) TestWriteFile(c *tc.C) {
	writeTestControllersFile(c)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("controllers.yaml"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, testControllersYAML[1:])
}

func (s *ControllersFileSuite) TestReadNoFile(c *tc.C) {
	controllers, err := jujuclient.ReadControllersFile("nohere.yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllers, tc.NotNil)
	c.Assert(controllers.Controllers, tc.HasLen, 0)
	c.Assert(controllers.CurrentController, tc.Equals, "")
}

func (s *ControllersFileSuite) TestReadPermissionsError(c *tc.C) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file-%d", time.Now().UnixNano()))
	err := os.WriteFile(path, []byte(""), 0377)
	c.Assert(err, tc.ErrorIsNil)

	_, err = jujuclient.ReadControllersFile(path)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "open .*: permission denied")
}

func (s *ControllersFileSuite) TestReadEmptyFile(c *tc.C) {
	err := os.WriteFile(osenv.JujuXDGDataHomePath("controllers.yaml"), []byte(""), 0600)
	c.Assert(err, tc.ErrorIsNil)

	controllerStore := jujuclient.NewFileClientStore()
	controllers, err := controllerStore.AllControllers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllers, tc.IsNil)
}

func parseControllers(c *tc.C) *jujuclient.Controllers {
	controllers, err := jujuclient.ParseControllers([]byte(testControllersYAML))
	c.Assert(err, tc.ErrorIsNil)

	// ensure that multiple server hostnames and eapi endpoints are parsed correctly
	c.Assert(controllers.Controllers["mallards"].APIEndpoints, tc.HasLen, 2)
	return controllers
}

func writeTestControllersFile(c *tc.C) *jujuclient.Controllers {
	controllers := parseControllers(c)
	err := jujuclient.WriteControllersFile(controllers)
	c.Assert(err, tc.ErrorIsNil)
	return controllers
}

func (s *ControllersFileSuite) TestParseControllerMetadata(c *tc.C) {
	controllers := parseControllers(c)
	var names []string
	for name := range controllers.Controllers {
		names = append(names, name)
	}
	c.Assert(names, tc.SameContents,
		[]string{"mark-test-prodstack", "mallards", "aws-test"},
	)
	c.Assert(controllers.CurrentController, tc.Equals, "mallards")
}

func (s *ControllersFileSuite) TestParseControllerMetadataError(c *tc.C) {
	controllers, err := jujuclient.ParseControllers([]byte("fail me now"))
	c.Assert(err, tc.ErrorMatches, "cannot unmarshal yaml controllers metadata: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `fail me...` into jujuclient.Controllers")
	c.Assert(controllers, tc.IsNil)
}

func (s *ControllersFileSuite) TestControllerFileOldFormat(c *tc.C) {
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
has-controller-changed-on-previous-switch: false
`
	modelCount := `
        model-count: 2`
	fileName := "controllers.yaml"

	// Contains model-count.
	err := os.WriteFile(osenv.JujuXDGDataHomePath(fileName), []byte(fmt.Sprintf(fileContent, modelCount)[1:]), 0600)
	c.Assert(err, tc.ErrorIsNil)

	controllerStore := jujuclient.NewFileClientStore()
	controllers, err := controllerStore.AllControllers()
	c.Assert(err, tc.ErrorIsNil)

	expectedDetails := jujuclient.ControllerDetails{
		ControllerUUID: "this-is-the-aws-test-uuid",
		APIEndpoints:   []string{"this-is-aws-test-of-many-api-endpoints"},
		DNSCache:       map[string][]string{"example.com": {"0.1.1.1", "0.2.2.2"}},
		CACert:         "this-is-aws-test-ca-cert",
		Cloud:          "aws",
		CloudRegion:    "us-east-1",
	}
	c.Assert(controllers, tc.DeepEquals, map[string]jujuclient.ControllerDetails{
		"aws-test": expectedDetails,
	})

	err = controllerStore.UpdateController("aws-test", expectedDetails)
	c.Assert(err, tc.ErrorIsNil)

	data, err := os.ReadFile(osenv.JujuXDGDataHomePath(fileName))
	c.Assert(err, tc.ErrorIsNil)

	// Has no model-count reference.
	c.Assert(string(data), tc.Equals, fmt.Sprintf(fileContent, "")[1:])
}
