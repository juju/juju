// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager_test

import (
	"fmt"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/systemmanager"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type systemManagerSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper
}

var _ = gc.Suite(&systemManagerSuite{})

func (s *systemManagerSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.JES)
	s.JujuConnSuite.SetUpTest(c)
}

func (s *systemManagerSuite) OpenAPI(c *gc.C) *systemmanager.Client {
	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })
	return systemmanager.NewClient(conn)
}

func (s *systemManagerSuite) TestAllEnvironments(c *gc.C) {
	owner := names.NewUserTag("user@remote")
	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "first", Owner: owner}).Close()
	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "second", Owner: owner}).Close()

	sysManager := s.OpenAPI(c)
	envs, err := sysManager.AllEnvironments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envs, gc.HasLen, 3)

	var obtained []string
	for _, env := range envs {
		obtained = append(obtained, fmt.Sprintf("%s/%s", env.Owner, env.Name))
	}
	expected := []string{
		"dummy-admin@local/dummyenv",
		"user@remote/first",
		"user@remote/second",
	}
	c.Assert(obtained, jc.SameContents, expected)
}

func (s *systemManagerSuite) TestEnvironmentConfig(c *gc.C) {
	sysManager := s.OpenAPI(c)
	env, err := sysManager.EnvironmentConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env["name"], gc.Equals, "dummyenv")
}

func (s *systemManagerSuite) TestDestroySystem(c *gc.C) {
	s.Factory.MakeEnvironment(c, &factory.EnvParams{Name: "foo"}).Close()

	sysManager := s.OpenAPI(c)
	err := sysManager.DestroySystem(false, false)
	c.Assert(err, gc.ErrorMatches, "state server environment cannot be destroyed before all other environments are destroyed")
}

func (s *systemManagerSuite) TestListBlockedEnvironments(c *gc.C) {
	err := s.State.SwitchBlockOn(state.ChangeBlock, "change block for state server")
	err = s.State.SwitchBlockOn(state.DestroyBlock, "destroy block for state server")
	c.Assert(err, jc.ErrorIsNil)

	sysManager := s.OpenAPI(c)
	results, err := sysManager.ListBlockedEnvironments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []params.EnvironmentBlockInfo{
		params.EnvironmentBlockInfo{
			Name:     "dummyenv",
			UUID:     s.State.EnvironUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockChange",
				"BlockDestroy",
			},
		},
	})
}

func (s *systemManagerSuite) TestRemoveBlocks(c *gc.C) {
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	sysManager := s.OpenAPI(c)
	err := sysManager.RemoveBlocks()
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *systemManagerSuite) TestWatchAllEnvs(c *gc.C) {
	// The WatchAllEnvs infrastructure is comprehensively tested
	// else. This test just ensure that the API calls work end-to-end.
	sysManager := s.OpenAPI(c)

	w, err := sysManager.WatchAllEnvs()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := w.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()

	deltasC := make(chan []multiwatcher.Delta)
	go func() {
		deltas, err := w.Next()
		c.Assert(err, jc.ErrorIsNil)
		deltasC <- deltas
	}()

	select {
	case deltas := <-deltasC:
		c.Assert(deltas, gc.HasLen, 1)
		envInfo := deltas[0].Entity.(*multiwatcher.EnvironmentInfo)

		env, err := s.State.Environment()
		c.Assert(err, jc.ErrorIsNil)

		c.Assert(envInfo.EnvUUID, gc.Equals, env.UUID())
		c.Assert(envInfo.Name, gc.Equals, env.Name())
		c.Assert(envInfo.Life, gc.Equals, multiwatcher.Life("alive"))
		c.Assert(envInfo.Owner, gc.Equals, env.Owner().Id())
		c.Assert(envInfo.ServerUUID, gc.Equals, env.ServerUUID())
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}
