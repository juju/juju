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
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type DestroySuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api         *fakeDestroyAPI
	configstore configstore.Storage
	store       jujuclient.ClientStore
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

	var err error
	s.configstore, err = configstore.Default()
	c.Assert(err, jc.ErrorIsNil)

	var envList = []struct {
		name       string
		serverUUID string
		modelUUID  string
	}{
		{
			name:       "test1:test1",
			serverUUID: "test1-uuid",
			modelUUID:  "test1-uuid",
		}, {
			name:       "test1:test2",
			serverUUID: "test1-uuid",
			modelUUID:  "test2-uuid",
		},
	}
	for _, env := range envList {
		info := s.configstore.CreateInfo(env.name)
		info.SetAPIEndpoint(configstore.APIEndpoint{
			Addresses:  []string{"localhost"},
			CACert:     testing.CACert,
			ModelUUID:  env.modelUUID,
			ServerUUID: env.serverUUID,
		})

		err := info.Write()
		c.Assert(err, jc.ErrorIsNil)
	}

	err = modelcmd.WriteCurrentController("test1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DestroySuite) runDestroyCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := model.NewDestroyCommandForTest(s.api, s.store)
	return testing.RunCommand(c, cmd, args...)
}

func (s *DestroySuite) NewDestroyCommand() cmd.Command {
	return model.NewDestroyCommandForTest(s.api, s.store)
}

func checkEnvironmentExistsInStore(c *gc.C, name string, store configstore.Storage) {
	_, err := store.ReadInfo(name)
	c.Assert(err, jc.ErrorIsNil)
}

func checkEnvironmentRemovedFromStore(c *gc.C, name string, store configstore.Storage) {
	_, err := store.ReadInfo(name)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DestroySuite) TestDestroyNoEnvironmentNameError(c *gc.C) {
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

func (s *DestroySuite) TestDestroyUnknownEnvironment(c *gc.C) {
	_, err := s.runDestroyCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot read model info: model "test1:foo" not found`)
}

func (s *DestroySuite) TestDestroyCannotConnectToAPI(c *gc.C) {
	s.api.err = errors.New("connection refused")
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy model: connection refused")
	c.Check(c.GetTestLog(), jc.Contains, "failed to destroy model \"test2\"")
	checkEnvironmentExistsInStore(c, "test1:test2", s.configstore)
}

func (s *DestroySuite) TestSystemDestroyFails(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, `"test1" is a controller; use 'juju destroy-controller' to destroy it`)
	checkEnvironmentExistsInStore(c, "test1:test1", s.configstore)
}

func (s *DestroySuite) TestDestroy(c *gc.C) {
	checkEnvironmentExistsInStore(c, "test1:test2", s.configstore)
	_, err := s.runDestroyCommand(c, "test2", "-y")
	c.Assert(err, jc.ErrorIsNil)
	checkEnvironmentRemovedFromStore(c, "test1:test2", s.configstore)
}

func (s *DestroySuite) TestFailedDestroyEnvironment(c *gc.C) {
	s.api.err = errors.New("permission denied")
	_, err := s.runDestroyCommand(c, "test1:test2", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy model: permission denied")
	checkEnvironmentExistsInStore(c, "test1:test2", s.configstore)
}

func (s *DestroySuite) resetEnvironment(c *gc.C) {
	info := s.configstore.CreateInfo("test1:test2")
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:  []string{"localhost"},
		CACert:     testing.CACert,
		ModelUUID:  "test2-uuid",
		ServerUUID: "test1-uuid",
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
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
	checkEnvironmentExistsInStore(c, "test1:test1", s.configstore)

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
	checkEnvironmentExistsInStore(c, "test1:test2", s.configstore)

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
		checkEnvironmentRemovedFromStore(c, "test1:test2", s.configstore)

		// Add the test2 environment back into the store for the next test
		s.resetEnvironment(c)
	}
}

func (s *DestroySuite) TestBlockedDestroy(c *gc.C) {
	s.api.err = &params.Error{Code: params.CodeOperationBlocked}
	s.runDestroyCommand(c, "test2", "-y")
	c.Check(c.GetTestLog(), jc.Contains, "To remove the block")
}
