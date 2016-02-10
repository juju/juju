// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"fmt"
	"regexp"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ShowControllerSuite struct {
	baseControllerSuite
}

var _ = gc.Suite(&ShowControllerSuite{})

func (s *ShowControllerSuite) TestShowOneControllerOneInStore(c *gc.C) {
	s.controllers = []testControllers{{
		"test1",
		"uuid.1",
		"ca.cert.1",
	}}
	s.createMemClientStore(c)

	s.expectedOutput = `
test1:
  servers: []
  uuid: uuid.1
  api-endpoints: []
  ca-cert: ca.cert.1
`[1:]

	s.assertShowController(c, "test1")
}

func (s *ShowControllerSuite) TestShowOneControllerManyInStore(c *gc.C) {
	s.createMemClientStore(c)

	s.expectedOutput = `
test1:
  servers: []
  uuid: uuid.1
  api-endpoints: []
  ca-cert: ca.cert.1
`[1:]
	s.assertShowController(c, "test1")
}

func (s *ShowControllerSuite) TestShowSomeControllerMoreInStore(c *gc.C) {
	s.createMemClientStore(c)
	s.expectedOutput = `
abc:
  servers: []
  uuid: uuid.2
  api-endpoints: []
  ca-cert: ca.cert.2
test1:
  servers: []
  uuid: uuid.1
  api-endpoints: []
  ca-cert: ca.cert.1
`[1:]

	s.assertShowController(c, "test1", "abc")
}

func (s *ShowControllerSuite) TestShowControllerJsonOne(c *gc.C) {
	s.controllers = []testControllers{{
		"test1",
		"uuid.1",
		"ca.cert.1",
	}}
	s.createMemClientStore(c)

	s.expectedOutput = `
{"test1":{"Servers":null,"ControllerUUID":"uuid.1","APIEndpoints":null,"CACert":"ca.cert.1"}}
`[1:]

	s.assertShowController(c, "--format", "json", "test1")
}

func (s *ShowControllerSuite) TestShowControllerJsonMany(c *gc.C) {
	s.createMemClientStore(c)
	s.expectedOutput = `
{"abc":{"Servers":null,"ControllerUUID":"uuid.2","APIEndpoints":null,"CACert":"ca.cert.2"},"test1":{"Servers":null,"ControllerUUID":"uuid.1","APIEndpoints":null,"CACert":"ca.cert.1"}}
`[1:]
	s.assertShowController(c, "--format", "json", "test1", "abc")
}

func (s *ShowControllerSuite) TestShowControllerAccessStoreErr(c *gc.C) {
	msg := "my bad"
	s.storeAccess = func() (jujuclient.ControllerStore, error) {
		return nil, errors.New(msg)
	}

	s.expectedErr = fmt.Sprintf("failed to get jujuclient store: %v", msg)

	s.assertShowControllerFailed(c, "test1")
}

func (s *ShowControllerSuite) TestShowControllerReadFromStoreErr(c *gc.C) {
	msg := "fail getting controller"
	s.store = &mockClientStore{msg}
	s.expectedErr = fmt.Sprintf("failed to get controller %q from jujuclient store: %v", "test1", msg)
	s.assertShowControllerFailed(c, "test1")
}

func (s *ShowControllerSuite) TestShowControllerNoArgs(c *gc.C) {
	s.createMemClientStore(c)
	s.expectedErr = regexp.QuoteMeta(`must specify controller name(s)`)
	s.assertShowControllerFailed(c)
}

func (s *ShowControllerSuite) TestShowControllerNotFound(c *gc.C) {
	s.controllers = nil
	s.createMemClientStore(c)

	s.expectedErr = regexp.QuoteMeta(`failed to get controller "whoops" from jujuclient store: controller whoops not found`)
	s.assertShowControllerFailed(c, "whoops")
}

func (s *ShowControllerSuite) TestShowControllerUnrecognizedFlag(c *gc.C) {
	s.createMemClientStore(c)

	// m (model) is not a valid flag for this command \o/
	s.expectedErr = `flag provided but not defined: -m`
	s.assertShowControllerFailed(c, "-m", "my.world")
}

func (s *ShowControllerSuite) TestShowControllerUnrecognizedOptionFlag(c *gc.C) {
	s.createMemClientStore(c)

	// model is not a valid option flag for this command \o/
	s.expectedErr = `flag provided but not defined: --model`
	s.assertShowControllerFailed(c, "--model", "still.my.world")
}

func (s *ShowControllerSuite) runShowController(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, controller.NewShowControllerCommandForTest(s.storeAccess), args...)
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
