// Copyright 2015,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type ListControllersSuite struct {
	store       jujuclient.ControllerStore
	storeAccess func() (jujuclient.ControllerStore, error)
}

var _ = gc.Suite(&ListControllersSuite{})

func (s *ListControllersSuite) SetUpTest(c *gc.C) {
	s.storeAccess = func() (jujuclient.ControllerStore, error) {
		return s.store, nil
	}
}

func (s *ListControllersSuite) TestListControllers(c *gc.C) {
	s.createMemClientStore(c)
	s.assertListControllers(c, `
CONTROLLER       MODEL  USER  SERVER
abc                           
controller.name               
test1                         

`[1:])
}

func (s *ListControllersSuite) TestListControllersYaml(c *gc.C) {
	s.createMemClientStore(c)
	s.assertListControllers(c, `
- controller: abc
- controller: controller.name
- controller: test1
`[1:],
		"--format", "yaml")
}

func (s *ListControllersSuite) TestListControllersJson(c *gc.C) {
	s.createMemClientStore(c)
	s.assertListControllers(c, `
[{"controller":"abc"},{"controller":"controller.name"},{"controller":"test1"}]
`[1:],
		"--format", "json")
}

func (s *ListControllersSuite) TestListControllersAccessStoreErr(c *gc.C) {
	msg := "my bad"
	s.storeAccess = func() (jujuclient.ControllerStore, error) {
		return nil, errors.New(msg)
	}
	s.assertListControllersFailed(c, fmt.Sprintf("failed to get jujuclient store: %v", msg))
}

func (s *ListControllersSuite) TestListControllersReadFromStoreErr(c *gc.C) {
	msg := "fail getting all controllers"
	s.store = &mockStore{msg}
	s.assertListControllersFailed(c, fmt.Sprintf("failed to list controllers in jujuclient store: %v", msg))
}

func (s *ListControllersSuite) TestListControllersUnrecognizedArg(c *gc.C) {
	s.createMemClientStore(c)
	s.assertListControllersFailed(c, `unrecognized args: \["whoops"\]`, "whoops")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedFlag(c *gc.C) {
	s.createMemClientStore(c)
	// m (model) is not a valid flag for this command \o/
	s.assertListControllersFailed(c, `flag provided but not defined: -m`, "-m", "my.world")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedOptionFlag(c *gc.C) {
	s.createMemClientStore(c)
	// model is not a valid option flag for this command \o/
	s.assertListControllersFailed(c, `flag provided but not defined: --model`, "--model", "still.my.world")
}

func (s *ListControllersSuite) assertListControllersFailed(c *gc.C, msg string, args ...string) {
	_, err := testing.RunCommand(c, controller.NewListControllersCommandForTest(s.storeAccess), args...)
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *ListControllersSuite) assertListControllers(c *gc.C, output string, args ...string) {
	context, err := testing.RunCommand(c, controller.NewListControllersCommandForTest(s.storeAccess), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, output)
}

func (s *ListControllersSuite) createMemClientStore(c *gc.C) {
	s.store = jujuclienttesting.NewMemControllerStore()

	controllers := []struct {
		name           string
		controllerUUID string
		caCert         string
	}{
		{
			"test1",
			"uuid.1",
			"ca.cert.1",
		},
		{
			"abc",
			"uuid.2",
			"ca.cert.2",
		},
		{
			"controller.name",
			"uuid.3",
			"ca.cert.3",
		},
	}
	for _, controller := range controllers {
		err := s.store.UpdateController(controller.name,
			jujuclient.ControllerDetails{
				ControllerUUID: controller.controllerUUID,
				CACert:         controller.caCert,
			})
		c.Assert(err, jc.ErrorIsNil)
	}
}

type mockStore struct {
	msg string
}

// AllControllers implements ControllersGetter.AllControllers
func (c *mockStore) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	return nil, errors.New(c.msg)
}

// ControllerByName implements ControllersGetter.ControllerByName
func (c *mockStore) ControllerByName(name string) (*jujuclient.ControllerDetails, error) {
	panic("not for test")
}

// UpdateController implements ControllersUpdater.UpdateController
func (c *mockStore) UpdateController(name string, one jujuclient.ControllerDetails) error {
	panic("not for test")
}

// RemoveController implements ControllersRemover.RemoveController
func (c *mockStore) RemoveController(name string) error {
	panic("not for test")
}
