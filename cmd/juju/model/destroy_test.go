// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/modelcmd"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type DestroySuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeDestroyAPI
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&DestroySuite{})

// fakeDestroyAPI mocks out the cient API
type fakeDestroyAPI struct {
	err error
	env map[string]interface{}
}

func (f *fakeDestroyAPI) Close() error { return nil }

func (f *fakeDestroyAPI) DestroyModel() error {
	return f.err
}

func (s *DestroySuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &fakeDestroyAPI{}
	s.api.err = nil

	err := modelcmd.WriteCurrentController("test1")
	c.Assert(err, jc.ErrorIsNil)
	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["test1"] = jujuclient.ControllerDetails{ControllerUUID: "test1-uuid"}
	s.store.Models["test1"] = jujuclient.ControllerAccountModels{
		AccountModels: map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{
					"test1": {"test1-uuid"},
					"test2": {"test2-uuid"},
				},
			},
		},
	}
	s.store.Accounts["test1"] = &jujuclient.ControllerAccounts{
		CurrentAccount: "admin@local",
	}
}

func (s *DestroySuite) runDestroyCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := model.NewDestroyCommandForTest(s.api, s.store)
	return testing.RunCommand(c, cmd, args...)
}

func (s *DestroySuite) NewDestroyCommand() cmd.Command {
	return model.NewDestroyCommandForTest(s.api, s.store)
}

func checkModelExistsInStore(c *gc.C, name string, store jujuclient.ClientStore) {
	controller, model := modelcmd.SplitModelName(name)
	_, err := store.ModelByName(controller, "admin@local", model)
	c.Assert(err, jc.ErrorIsNil)
}

func checkModelRemovedFromStore(c *gc.C, name string, store jujuclient.ClientStore) {
	controller, model := modelcmd.SplitModelName(name)
	_, err := store.ModelByName(controller, "admin@local", model)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DestroySuite) TestDestroyNoModelNameError(c *gc.C) {
	_, err := s.runDestroyCommand(c)
	c.Assert(err, gc.ErrorMatches, "no model specified")
}

func (s *DestroySuite) TestDestroyBadFlags(c *gc.C) {
	_, err := s.runDestroyCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -n")
}

func (s *DestroySuite) TestDestroyUnknownArgument(c *gc.C) {
	_, err := s.runDestroyCommand(c, "model", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *DestroySuite) TestDestroyUnknownModel(c *gc.C) {
	_, err := s.runDestroyCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot read model info: model test1:admin@local:foo not found`)
}

func (s *DestroySuite) TestDestroyCannotConnectToAPI(c *gc.C) {
	s.api.err = errors.New("connection refused")
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy model: connection refused")
	c.Check(c.GetTestLog(), jc.Contains, "failed to destroy model \"test2\"")
	checkModelExistsInStore(c, "test1:test2", s.store)
}

func (s *DestroySuite) TestSystemDestroyFails(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, `"test1" is a controller; use 'juju destroy-controller' to destroy it`)
	checkModelExistsInStore(c, "test1:test1", s.store)
}

func (s *DestroySuite) TestDestroy(c *gc.C) {
	checkModelExistsInStore(c, "test1:test2", s.store)
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
	checkModelRemovedFromStore(c, "test1:test2", s.store)
}

func (s *DestroySuite) TestFailedDestroyModel(c *gc.C) {
	s.api.err = errors.New("permission denied")
	_, err := s.runDestroyCommand(c, "test1:test2", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy model: permission denied")
	checkModelExistsInStore(c, "test1:test2", s.store)
}

func (s *DestroySuite) resetModel(c *gc.C) {
	s.store.Models["test1"] = jujuclient.ControllerAccountModels{
		AccountModels: map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{
					"test1": {"test1-uuid"},
					"test2": {"test2-uuid"},
				},
			},
		},
	}
}

func (s *DestroySuite) TestDestroyCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	_, errc := cmdtesting.RunCommand(ctx, s.NewDestroyCommand(), "test2")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "model destruction: aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test2(.|\n)*")
	checkModelExistsInStore(c, "test1:test1", s.store)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	_, errc = cmdtesting.RunCommand(ctx, s.NewDestroyCommand(), "test2")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "model destruction: aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test2(.|\n)*")
	checkModelExistsInStore(c, "test1:test2", s.store)

	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		stdin.Reset()
		stdout.Reset()
		stdin.WriteString(answer)
		_, errc = cmdtesting.RunCommand(ctx, s.NewDestroyCommand(), "test2")
		select {
		case err := <-errc:
			c.Check(err, jc.ErrorIsNil)
		case <-time.After(testing.LongWait):
			c.Fatalf("command took too long")
		}
		checkModelRemovedFromStore(c, "test1:test2", s.store)

		// Add the test2 model back into the store for the next test
		s.resetModel(c)
	}
}

func (s *DestroySuite) TestBlockedDestroy(c *gc.C) {
	s.api.err = &params.Error{Code: params.CodeOperationBlocked}
	s.runDestroyCommand(c, "test2", "-y")
	c.Check(c.GetTestLog(), jc.Contains, "To remove the block")
}
