// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/system"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type DestroySuite struct {
	testing.FakeJujuHomeSuite
	api       *fakeDestroyAPI
	clientapi *fakeDestroyAPIClient
	store     configstore.Storage
	apierror  error
}

var _ = gc.Suite(&DestroySuite{})

// fakeDestroyAPI mocks out the systemmanager API
type fakeDestroyAPI struct {
	err          error
	env          map[string]interface{}
	destroyAll   bool
	ignoreBlocks bool
	blocks       []params.EnvironmentBlockInfo
	blocksErr    error
}

func (f *fakeDestroyAPI) Close() error { return nil }

func (f *fakeDestroyAPI) EnvironmentConfig() (map[string]interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.env, nil
}

func (f *fakeDestroyAPI) DestroySystem(destroyAll bool, ignoreBlocks bool) error {
	f.destroyAll = destroyAll
	f.ignoreBlocks = ignoreBlocks
	return f.err
}

func (f *fakeDestroyAPI) ListBlockedEnvironments() ([]params.EnvironmentBlockInfo, error) {
	return f.blocks, f.blocksErr
}

// fakeDestroyAPIClient mocks out the client API
type fakeDestroyAPIClient struct {
	err           error
	env           map[string]interface{}
	envgetcalled  bool
	destroycalled bool
}

func (f *fakeDestroyAPIClient) Close() error { return nil }

func (f *fakeDestroyAPIClient) EnvironmentGet() (map[string]interface{}, error) {
	f.envgetcalled = true
	if f.err != nil {
		return nil, f.err
	}
	return f.env, nil
}

func (f *fakeDestroyAPIClient) DestroyEnvironment() error {
	f.destroycalled = true
	return f.err
}

func createBootstrapInfo(c *gc.C, name string) map[string]interface{} {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type":         "dummy",
		"name":         name,
		"state-server": "true",
		"state-id":     "1",
	})
	c.Assert(err, jc.ErrorIsNil)
	return cfg.AllAttrs()
}

func (s *DestroySuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.clientapi = &fakeDestroyAPIClient{}
	s.api = &fakeDestroyAPI{}
	s.apierror = nil

	var err error
	s.store, err = configstore.Default()
	c.Assert(err, jc.ErrorIsNil)

	var envList = []struct {
		name         string
		serverUUID   string
		envUUID      string
		bootstrapCfg map[string]interface{}
	}{
		{
			name:         "test1",
			serverUUID:   "test1-uuid",
			envUUID:      "test1-uuid",
			bootstrapCfg: createBootstrapInfo(c, "test1"),
		}, {
			name:       "test2",
			serverUUID: "test1-uuid",
			envUUID:    "test2-uuid",
		}, {
			name:    "test3",
			envUUID: "test3-uuid",
		},
	}
	for _, env := range envList {
		info := s.store.CreateInfo(env.name)
		info.SetAPIEndpoint(configstore.APIEndpoint{
			Addresses:   []string{"localhost"},
			CACert:      testing.CACert,
			EnvironUUID: env.envUUID,
			ServerUUID:  env.serverUUID,
		})

		if env.bootstrapCfg != nil {
			info.SetBootstrapConfig(env.bootstrapCfg)
		}
		err := info.Write()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *DestroySuite) runDestroyCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := system.NewDestroyCommand(s.api, s.clientapi, s.apierror)
	return testing.RunCommand(c, cmd, args...)
}

func (s *DestroySuite) newDestroyCommand() *system.DestroyCommand {
	return system.NewDestroyCommand(s.api, s.clientapi, s.apierror)
}

func checkSystemExistsInStore(c *gc.C, name string, store configstore.Storage) {
	_, err := store.ReadInfo(name)
	c.Check(err, jc.ErrorIsNil)
}

func checkSystemRemovedFromStore(c *gc.C, name string, store configstore.Storage) {
	_, err := store.ReadInfo(name)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DestroySuite) TestDestroyNoSystemNameError(c *gc.C) {
	_, err := s.runDestroyCommand(c)
	c.Assert(err, gc.ErrorMatches, "no system specified")
}

func (s *DestroySuite) TestDestroyBadFlags(c *gc.C) {
	_, err := s.runDestroyCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -n")
}

func (s *DestroySuite) TestDestroyUnknownArgument(c *gc.C) {
	_, err := s.runDestroyCommand(c, "environment", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *DestroySuite) TestDestroyUnknownSystem(c *gc.C) {
	_, err := s.runDestroyCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot read system info: environment "foo" not found`)
}

func (s *DestroySuite) TestDestroyNonSystemEnvFails(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test2")
	c.Assert(err, gc.ErrorMatches, "\"test2\" is not a system; use juju environment destroy to destroy it")
}

func (s *DestroySuite) TestDestroySystemNotFoundNotRemovedFromStore(c *gc.C) {
	s.apierror = errors.NotFoundf("test1")
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot connect to API: test1 not found")
	c.Check(c.GetTestLog(), jc.Contains, "If the system is unusable")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyCannotConnectToAPI(c *gc.C) {
	s.apierror = errors.New("connection refused")
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot connect to API: connection refused")
	c.Check(c.GetTestLog(), jc.Contains, "If the system is unusable")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroy(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.ignoreBlocks, jc.IsFalse)
	c.Assert(s.api.destroyAll, jc.IsFalse)
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyWithDestroyAllEnvsFlag(c *gc.C) {
	_, err := s.runDestroyCommand(c, "test1", "-y", "--destroy-all-environments")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.ignoreBlocks, jc.IsFalse)
	c.Assert(s.api.destroyAll, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestDestroyEnvironmentGetFails(c *gc.C) {
	s.api.err = errors.NotFoundf(`system "test3"`)
	_, err := s.runDestroyCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot obtain bootstrap information: system \"test3\" not found")
	checkSystemExistsInStore(c, "test3", s.store)
}

func (s *DestroySuite) TestDestroyFallsBackToClient(c *gc.C) {
	s.api.err = &params.Error{"DestroyEnvironment", params.CodeNotImplemented}
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *DestroySuite) TestEnvironmentGetFallsBackToClient(c *gc.C) {
	s.api.err = &params.Error{"EnvironmentGet", params.CodeNotImplemented}
	s.clientapi.env = createBootstrapInfo(c, "test3")
	_, err := s.runDestroyCommand(c, "test3", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.envgetcalled, jc.IsTrue)
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test3", s.store)
}

func (s *DestroySuite) TestFailedDestroyEnvironment(c *gc.C) {
	s.api.err = errors.New("permission denied")
	_, err := s.runDestroyCommand(c, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy system: permission denied")
	c.Assert(s.api.ignoreBlocks, jc.IsFalse)
	c.Assert(s.api.destroyAll, jc.IsFalse)
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *DestroySuite) resetSystem(c *gc.C) {
	info := s.store.CreateInfo("test1")
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   []string{"localhost"},
		CACert:      testing.CACert,
		EnvironUUID: "test1-uuid",
		ServerUUID:  "test1-uuid",
	})
	info.SetBootstrapConfig(createBootstrapInfo(c, "test1"))
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
	_, errc := cmdtesting.RunCommand(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "system destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkSystemExistsInStore(c, "test1", s.store)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	_, errc = cmdtesting.RunCommand(ctx, s.newDestroyCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "system destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkSystemExistsInStore(c, "test1", s.store)

	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		stdin.Reset()
		stdout.Reset()
		stdin.WriteString(answer)
		_, errc = cmdtesting.RunCommand(ctx, s.newDestroyCommand(), "test1")
		select {
		case err := <-errc:
			c.Check(err, jc.ErrorIsNil)
		case <-time.After(testing.LongWait):
			c.Fatalf("command took too long")
		}
		checkSystemRemovedFromStore(c, "test1", s.store)

		// Add the test1 system back into the store for the next test
		s.resetSystem(c)
	}
}

func (s *DestroySuite) TestBlockedDestroy(c *gc.C) {
	s.api.err = &params.Error{Code: params.CodeOperationBlocked}
	s.runDestroyCommand(c, "test1", "-y")
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, "To remove all blocks in the system, please run:")
	c.Check(testLog, jc.Contains, "juju system remove-blocks")
}

func (s *DestroySuite) TestDestroyListBlocksError(c *gc.C) {
	s.api.err = &params.Error{Code: params.CodeOperationBlocked}
	s.api.blocksErr = errors.New("unexpected api error")
	s.runDestroyCommand(c, "test1", "-y")
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, "To remove all blocks in the system, please run:")
	c.Check(testLog, jc.Contains, "juju system remove-blocks")
	c.Check(testLog, jc.Contains, "Unable to list blocked environments: unexpected api error")
}

func (s *DestroySuite) TestDestroyReturnsBlocks(c *gc.C) {
	s.api.err = &params.Error{Code: params.CodeOperationBlocked}
	s.api.blocks = []params.EnvironmentBlockInfo{
		params.EnvironmentBlockInfo{
			Name:     "test1",
			UUID:     "test1-uuid",
			OwnerTag: "cheryl@local",
			Blocks: []string{
				"BlockDestroy",
			},
		},
		params.EnvironmentBlockInfo{
			Name:     "test2",
			UUID:     "test2-uuid",
			OwnerTag: "bob@local",
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	}
	ctx, _ := s.runDestroyCommand(c, "test1", "-y", "--destroy-all-environments")
	c.Assert(testing.Stderr(ctx), gc.Equals, ""+
		"NAME   ENVIRONMENT UUID  OWNER         BLOCKS\n"+
		"test1  test1-uuid        cheryl@local  destroy-environment\n"+
		"test2  test2-uuid        bob@local     destroy-environment,all-changes\n")
}
