// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"regexp"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type ShowControllerSuite struct {
	baseControllerSuite
}

var _ = gc.Suite(&ShowControllerSuite{})

func (s *ShowControllerSuite) TestShowOneControllerOneInStore(c *gc.C) {
	s.controllersYaml = `controllers:
  local.mallards:
    servers: [maas-1-05.cluster.mallards]
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
`
	s.createTestClientStore(c)

	s.expectedOutput = `
local.mallards:
  details:
    servers: [maas-1-05.cluster.mallards]
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
  accounts:
    admin@local:
      user: admin@local
      models:
        admin:
          uuid: abc
        my-model:
          uuid: def
      current-model: my-model
    bob@local:
      user: bob@local
    bob@remote:
      user: bob@remote
  current-account: admin@local
`[1:]

	s.assertShowController(c, "local.mallards")
}

func (s *ShowControllerSuite) TestShowControllerWithPasswords(c *gc.C) {
	s.controllersYaml = `controllers:
  local.mallards:
    servers: [maas-1-05.cluster.mallards]
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
`
	s.createTestClientStore(c)

	s.expectedOutput = `
local.mallards:
  details:
    servers: [maas-1-05.cluster.mallards]
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
  accounts:
    admin@local:
      user: admin@local
      password: hunter2
      models:
        admin:
          uuid: abc
        my-model:
          uuid: def
      current-model: my-model
    bob@local:
      user: bob@local
      password: huntert00
    bob@remote:
      user: bob@remote
  current-account: admin@local
`[1:]

	s.assertShowController(c, "local.mallards", "--show-passwords")
}

func (s *ShowControllerSuite) TestShowOneControllerManyInStore(c *gc.C) {
	s.createTestClientStore(c)

	s.expectedOutput = `
local.aws-test:
  details:
    servers: [instance-1-2-4.useast.aws.com]
    uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
  accounts:
    admin@local:
      user: admin@local
      models:
        admin:
          uuid: ghi
      current-model: admin
`[1:]
	s.assertShowController(c, "local.aws-test")
}

func (s *ShowControllerSuite) TestShowSomeControllerMoreInStore(c *gc.C) {
	s.createTestClientStore(c)
	s.expectedOutput = `
local.aws-test:
  details:
    servers: [instance-1-2-4.useast.aws.com]
    uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
  accounts:
    admin@local:
      user: admin@local
      models:
        admin:
          uuid: ghi
      current-model: admin
local.mark-test-prodstack:
  details:
    servers: [vm-23532.prodstack.canonical.com, great.test.server.hostname.co.nz]
    uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    ca-cert: this-is-a-ca-cert
  accounts:
    admin@local:
      user: admin@local
`[1:]

	s.assertShowController(c, "local.aws-test", "local.mark-test-prodstack")
}

func (s *ShowControllerSuite) TestShowControllerJsonOne(c *gc.C) {
	s.createTestClientStore(c)

	s.expectedOutput = `
{"local.aws-test":{"details":{"servers":["instance-1-2-4.useast.aws.com"],"uuid":"this-is-the-aws-test-uuid","api-endpoints":["this-is-aws-test-of-many-api-endpoints"],"ca-cert":"this-is-aws-test-ca-cert"},"accounts":{"admin@local":{"user":"admin@local","models":{"admin":{"uuid":"ghi"}},"current-model":"admin"}}}}
`[1:]

	s.assertShowController(c, "--format", "json", "local.aws-test")
}

func (s *ShowControllerSuite) TestShowControllerJsonMany(c *gc.C) {
	s.createTestClientStore(c)
	s.expectedOutput = `
{"local.aws-test":{"details":{"servers":["instance-1-2-4.useast.aws.com"],"uuid":"this-is-the-aws-test-uuid","api-endpoints":["this-is-aws-test-of-many-api-endpoints"],"ca-cert":"this-is-aws-test-ca-cert"},"accounts":{"admin@local":{"user":"admin@local","models":{"admin":{"uuid":"ghi"}},"current-model":"admin"}}},"local.mark-test-prodstack":{"details":{"servers":["vm-23532.prodstack.canonical.com","great.test.server.hostname.co.nz"],"uuid":"this-is-a-uuid","api-endpoints":["this-is-one-of-many-api-endpoints"],"ca-cert":"this-is-a-ca-cert"},"accounts":{"admin@local":{"user":"admin@local"}}}}
`[1:]
	s.assertShowController(c, "--format", "json", "local.aws-test", "local.mark-test-prodstack")
}

func (s *ShowControllerSuite) TestShowControllerReadFromStoreErr(c *gc.C) {
	s.createTestClientStore(c)

	msg := "fail getting controller"
	errStore := jujuclienttesting.NewStubStore()
	errStore.SetErrors(errors.New(msg))
	s.store = errStore
	s.expectedErr = msg

	s.assertShowControllerFailed(c, "test1")
	errStore.CheckCallNames(c, "ControllerByName")
}

func (s *ShowControllerSuite) TestShowControllerNoArgs(c *gc.C) {
	s.createTestClientStore(c)

	s.expectedOutput = `
{"local.aws-test":{"details":{"servers":["instance-1-2-4.useast.aws.com"],"uuid":"this-is-the-aws-test-uuid","api-endpoints":["this-is-aws-test-of-many-api-endpoints"],"ca-cert":"this-is-aws-test-ca-cert"},"accounts":{"admin@local":{"user":"admin@local","models":{"admin":{"uuid":"ghi"}},"current-model":"admin"}}}}
`[1:]
	err := modelcmd.WriteCurrentController("local.aws-test")
	c.Assert(err, jc.ErrorIsNil)
	s.assertShowController(c, "--format", "json")
}

func (s *ShowControllerSuite) TestShowControllerNoArgsNoCurrent(c *gc.C) {
	err := modelcmd.WriteCurrentController("")
	c.Assert(err, jc.ErrorIsNil)

	s.expectedErr = regexp.QuoteMeta(`there is no active controller`)
	s.assertShowControllerFailed(c)
}

func (s *ShowControllerSuite) TestShowControllerNotFound(c *gc.C) {
	s.createTestClientStore(c)

	s.expectedErr = `controller whoops not found`
	s.assertShowControllerFailed(c, "whoops")
}

func (s *ShowControllerSuite) TestShowControllerUnrecognizedFlag(c *gc.C) {
	s.expectedErr = `flag provided but not defined: -m`
	s.assertShowControllerFailed(c, "-m", "my.world")
}

func (s *ShowControllerSuite) TestShowControllerUnrecognizedOptionFlag(c *gc.C) {
	s.expectedErr = `flag provided but not defined: --model`
	s.assertShowControllerFailed(c, "--model", "still.my.world")
}

func (s *ShowControllerSuite) runShowController(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, controller.NewShowControllerCommandForTest(s.store), args...)
}

func (s *ShowControllerSuite) assertShowControllerFailed(c *gc.C, args ...string) {
	_, err := s.runShowController(c, args...)
	c.Assert(err, gc.ErrorMatches, s.expectedErr)
}

func (s *ShowControllerSuite) assertShowController(c *gc.C, args ...string) {
	context, err := s.runShowController(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, s.expectedOutput)
}
