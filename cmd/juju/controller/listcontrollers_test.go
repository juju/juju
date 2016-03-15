// Copyright 2015,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"encoding/json"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type ListControllersSuite struct {
	baseControllerSuite
}

var _ = gc.Suite(&ListControllersSuite{})

func (s *ListControllersSuite) TestListControllersEmptyStore(c *gc.C) {
	s.expectedOutput = `
CONTROLLER  MODEL  USER  SERVER

`[1:]

	s.store = jujuclienttesting.NewMemStore()
	s.assertListControllers(c)
}

func (s *ListControllersSuite) TestListControllers(c *gc.C) {
	s.expectedOutput = `
CONTROLLER                 MODEL     USER         SERVER
local.aws-test             -         -            instance-1-2-4.useast.aws.com
local.mallards*            my-model  admin@local  maas-1-05.cluster.mallards
local.mark-test-prodstack  -         -            vm-23532.prodstack.canonical.com

`[1:]

	s.createTestClientStore(c)
	s.assertListControllers(c)
}

func (s *ListControllersSuite) TestListControllersYaml(c *gc.C) {
	s.expectedOutput = `
controllers:
  local.aws-test:
    recent-server: instance-1-2-4.useast.aws.com
    servers: [instance-1-2-4.useast.aws.com]
    uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
  local.mallards:
    current-model: my-model
    user: admin@local
    recent-server: maas-1-05.cluster.mallards
    servers: [maas-1-05.cluster.mallards]
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
  local.mark-test-prodstack:
    recent-server: vm-23532.prodstack.canonical.com
    servers: [vm-23532.prodstack.canonical.com, great.test.server.hostname.co.nz]
    uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    ca-cert: this-is-a-ca-cert
current-controller: local.mallards
`[1:]

	s.createTestClientStore(c)
	s.assertListControllers(c, "--format", "yaml")
}

func (s *ListControllersSuite) TestListControllersJson(c *gc.C) {
	s.expectedOutput = ""
	s.createTestClientStore(c)
	jsonOut := s.assertListControllers(c, "--format", "json")
	var result controller.ControllerSet
	err := json.Unmarshal([]byte(jsonOut), &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, controller.ControllerSet{
		Controllers: map[string]controller.ControllerItem{
			"local.aws-test": {
				ControllerUUID: "this-is-the-aws-test-uuid",
				Server:         "instance-1-2-4.useast.aws.com",
				Servers:        []string{"instance-1-2-4.useast.aws.com"},
				APIEndpoints:   []string{"this-is-aws-test-of-many-api-endpoints"},
				CACert:         "this-is-aws-test-ca-cert",
			},
			"local.mallards": {
				ControllerUUID: "this-is-another-uuid",
				ModelName:      "my-model",
				User:           "admin@local",
				Server:         "maas-1-05.cluster.mallards",
				Servers:        []string{"maas-1-05.cluster.mallards"},
				APIEndpoints:   []string{"this-is-another-of-many-api-endpoints", "this-is-one-more-of-many-api-endpoints"},
				CACert:         "this-is-another-ca-cert",
			},
			"local.mark-test-prodstack": {
				ControllerUUID: "this-is-a-uuid",
				Server:         "vm-23532.prodstack.canonical.com",
				Servers:        []string{"vm-23532.prodstack.canonical.com", "great.test.server.hostname.co.nz"},
				APIEndpoints:   []string{"this-is-one-of-many-api-endpoints"},
				CACert:         "this-is-a-ca-cert",
			},
		},
		CurrentController: "local.mallards",
	})
}

func (s *ListControllersSuite) TestListControllersReadFromStoreErr(c *gc.C) {
	msg := "fail getting all controllers"
	errStore := jujuclienttesting.NewStubStore()
	errStore.SetErrors(errors.New(msg))
	s.store = errStore
	s.expectedErr = fmt.Sprintf("failed to list controllers: %v", msg)
	s.assertListControllersFailed(c)
	errStore.CheckCallNames(c, "AllControllers")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedArg(c *gc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `unrecognized args: \["whoops"\]`
	s.assertListControllersFailed(c, "whoops")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedFlag(c *gc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `flag provided but not defined: -m`
	s.assertListControllersFailed(c, "-m", "my.world")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedOptionFlag(c *gc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `flag provided but not defined: --model`
	s.assertListControllersFailed(c, "--model", "still.my.world")
}

func (s *ListControllersSuite) runListControllers(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, controller.NewListControllersCommandForTest(s.store), args...)
}

func (s *ListControllersSuite) assertListControllersFailed(c *gc.C, args ...string) {
	_, err := s.runListControllers(c, args...)
	c.Assert(err, gc.ErrorMatches, s.expectedErr)
}

func (s *ListControllersSuite) assertListControllers(c *gc.C, args ...string) string {
	context, err := s.runListControllers(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	output := testing.Stdout(context)
	if s.expectedOutput != "" {
		c.Assert(output, gc.Equals, s.expectedOutput)
	}
	return output
}
