// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type WhoAmITestSuite struct {
	testing.BaseSuite
	store          jujuclient.ClientStore
	expectedOutput string
	expectedErr    string
}

var _ = gc.Suite(&WhoAmITestSuite{})

func (s *WhoAmITestSuite) TestEmptyStore(c *gc.C) {
	s.expectedOutput = `
There is no current controller.
Run juju list-controllers to see available controllers.
`[1:]

	s.store = jujuclient.NewMemStore()
	s.assertWhoAmI(c)
}

func (s *WhoAmITestSuite) TestNoCurrentController(c *gc.C) {
	s.expectedOutput = `
There is no current controller.
Run juju list-controllers to see available controllers.
`[1:]

	s.store = &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
	}
	s.assertWhoAmI(c)
}

func (s *WhoAmITestSuite) TestNoCurrentModel(c *gc.C) {
	s.expectedOutput = `
Controller:  controller
Model:       <no-current-model>
User:        admin
`[1:]

	s.store = &jujuclient.MemStore{
		CurrentControllerName: "controller",
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		Models: map[string]*jujuclient.ControllerModels{
			"controller": {
				Models: map[string]jujuclient.ModelDetails{
					"admin/model": {ModelUUID: "model-uuid", ModelType: model.IAAS},
				},
			},
		},
		Accounts: map[string]jujuclient.AccountDetails{
			"controller": {
				User: "admin",
			},
		},
	}
	s.assertWhoAmI(c)
}

func (s *WhoAmITestSuite) TestNoCurrentUser(c *gc.C) {
	s.expectedOutput = `
You are not logged in to controller "controller" and model "admin/model".
Run juju login if you want to login.
`[1:]

	s.store = &jujuclient.MemStore{
		CurrentControllerName: "controller",
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		Models: map[string]*jujuclient.ControllerModels{
			"controller": {
				Models: map[string]jujuclient.ModelDetails{
					"admin/model": {ModelUUID: "model-uuid", ModelType: model.IAAS},
				},
				CurrentModel: "admin/model",
			},
		},
	}
	s.assertWhoAmI(c)
}

func (s *WhoAmITestSuite) assertWhoAmIForUser(c *gc.C, user, format string) {
	s.store = &jujuclient.MemStore{
		CurrentControllerName: "controller",
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		Models: map[string]*jujuclient.ControllerModels{
			"controller": {
				Models: map[string]jujuclient.ModelDetails{
					"admin/model": {ModelUUID: "model-uuid", ModelType: model.IAAS},
				},
				CurrentModel: "admin/model",
			},
		},
		Accounts: map[string]jujuclient.AccountDetails{
			"controller": {
				User: user,
			},
		},
	}
	s.assertWhoAmI(c, "--format", format)
}

func (s *WhoAmITestSuite) TestWhoAmISameUser(c *gc.C) {
	s.expectedOutput = `
Controller:  controller
Model:       model
User:        admin
`[1:]
	s.assertWhoAmIForUser(c, "admin", "tabular")
}

func (s *WhoAmITestSuite) TestWhoAmIYaml(c *gc.C) {
	s.expectedOutput = `
controller: controller
model: model
user: admin
`[1:]
	s.assertWhoAmIForUser(c, "admin", "yaml")
}

func (s *WhoAmITestSuite) TestWhoAmIJson(c *gc.C) {
	s.expectedOutput = `
{"controller":"controller","model":"model","user":"admin"}
`[1:]
	s.assertWhoAmIForUser(c, "admin", "json")
}

func (s *WhoAmITestSuite) TestWhoAmIDifferentUsersModel(c *gc.C) {
	s.expectedOutput = `
Controller:  controller
Model:       admin/model
User:        bob
`[1:]
	s.assertWhoAmIForUser(c, "bob", "tabular")
}

func (s *WhoAmITestSuite) TestFromStoreErr(c *gc.C) {
	msg := "fail getting current controller"
	errStore := jujuclienttesting.NewStubStore()
	errStore.SetErrors(errors.New(msg))
	s.store = errStore
	s.expectedErr = msg
	s.assertWhoAmIFailed(c)
	errStore.CheckCallNames(c, "CurrentController")
}

func (s *WhoAmITestSuite) runWhoAmI(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, user.NewWhoAmICommandForTest(s.store), args...)
}

func (s *WhoAmITestSuite) assertWhoAmIFailed(c *gc.C, args ...string) {
	_, err := s.runWhoAmI(c, args...)
	c.Assert(err, gc.ErrorMatches, s.expectedErr)
}

func (s *WhoAmITestSuite) assertWhoAmI(c *gc.C, args ...string) string {
	context, err := s.runWhoAmI(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stdout(context)
	if output == "" {
		output = cmdtesting.Stderr(context)
	}
	if s.expectedOutput != "" {
		c.Assert(output, gc.Equals, s.expectedOutput)
	}
	return output
}
