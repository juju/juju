// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager_test

import (
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
	"github.com/juju/juju/testing/factory"
)

type systemmanagerSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper
}

var _ = gc.Suite(&systemmanagerSuite{})

func (s *systemmanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *systemmanagerSuite) OpenAPI(c *gc.C) *systemmanager.Client {
	conn, err := juju.NewAPIState(s.AdminUserTag(c), s.Environ, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })
	return systemmanager.NewClient(conn)
}

func (s *systemmanagerSuite) TestEnvironmentGet(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	sysManager := s.OpenAPI(c)
	env, err := sysManager.EnvironmentGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env["name"], gc.Equals, "dummyenv")
}

func (s *systemmanagerSuite) TestDestroySystem(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	s.Factory.MakeEnvironment(c, &factory.EnvParams{Name: "foo"}).Close()

	sysManager := s.OpenAPI(c)
	err := sysManager.DestroySystem(names.NewEnvironTag(s.State.EnvironUUID()), false, false)
	c.Assert(err, gc.ErrorMatches, "state server environment cannot be destroyed before all other environments are destroyed")
}

func (s *systemmanagerSuite) TestListBlockedEnvironments(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	err := s.State.SwitchBlockOn(state.ChangeBlock, "change block for state server")
	err = s.State.SwitchBlockOn(state.DestroyBlock, "destroy block for state server")
	c.Assert(err, jc.ErrorIsNil)

	sysManager := s.OpenAPI(c)
	results, err := sysManager.ListBlockedEnvironments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []params.EnvironmentBlockInfo{
		params.EnvironmentBlockInfo{
			params.Environment{
				Name:     "dummyenv",
				UUID:     s.State.EnvironUUID(),
				OwnerTag: s.AdminUserTag(c).String(),
			},
			[]string{
				"BlockChange",
				"BlockDestroy",
			},
		},
	})
}
