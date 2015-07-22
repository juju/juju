// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/system"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ListBlocksSuite struct {
	testing.FakeJujuHomeSuite
	api      *fakeListBlocksAPI
	apierror error
}

var _ = gc.Suite(&ListBlocksSuite{})

// fakeListBlocksAPI mocks out the systemmanager API
type fakeListBlocksAPI struct {
	err    error
	blocks []params.EnvironmentBlockInfo
}

func (f *fakeListBlocksAPI) Close() error { return nil }

func (f *fakeListBlocksAPI) ListBlockedEnvironments() ([]params.EnvironmentBlockInfo, error) {
	return f.blocks, f.err
}

func (s *ListBlocksSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.apierror = nil
	s.api = &fakeListBlocksAPI{
		blocks: []params.EnvironmentBlockInfo{
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
		},
	}
}

func (s *ListBlocksSuite) runListBlocksCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := system.NewListBlocksCommand(s.api, s.apierror)
	return testing.RunCommand(c, cmd, args...)
}

func (s *ListBlocksSuite) TestListBlocksCannotConnectToAPI(c *gc.C) {
	s.apierror = errors.New("connection refused")
	_, err := s.runListBlocksCommand(c)
	c.Assert(err, gc.ErrorMatches, "cannot connect to the API: connection refused")
}

func (s *ListBlocksSuite) TestListBlocksError(c *gc.C) {
	s.api.err = errors.New("unexpected api error")
	s.runListBlocksCommand(c)
	testLog := c.GetTestLog()
	c.Check(testLog, jc.Contains, "Unable to list blocked environments: unexpected api error")
}

func (s *ListBlocksSuite) TestListBlocksTabular(c *gc.C) {
	ctx, err := s.runListBlocksCommand(c)
	c.Check(err, jc.ErrorIsNil)
	c.Check(testing.Stdout(ctx), gc.Equals, ""+
		"NAME   ENVIRONMENT UUID  OWNER         BLOCKS\n"+
		"test1  test1-uuid        cheryl@local  destroy-environment\n"+
		"test2  test2-uuid        bob@local     destroy-environment,all-changes\n"+
		"\n")
}

func (s *ListBlocksSuite) TestListBlocksJSON(c *gc.C) {
	ctx, err := s.runListBlocksCommand(c, "--format", "json")
	c.Check(err, jc.ErrorIsNil)
	c.Check(testing.Stdout(ctx), gc.Equals, "["+
		`{"name":"test1","env-uuid":"test1-uuid","owner-tag":"cheryl@local",`+
		`"blocks":["BlockDestroy"]},`+
		`{"name":"test2","env-uuid":"test2-uuid","owner-tag":"bob@local",`+
		`"blocks":["BlockDestroy","BlockChange"]}`+
		"]\n")
}

func (s *ListBlocksSuite) TestListBlocksYAML(c *gc.C) {
	ctx, err := s.runListBlocksCommand(c, "--format", "yaml")
	c.Check(err, jc.ErrorIsNil)
	c.Check(testing.Stdout(ctx), gc.Equals, ""+
		"- name: test1\n"+
		"  uuid: test1-uuid\n"+
		"  ownertag: cheryl@local\n"+
		"  blocks:\n"+
		"  - BlockDestroy\n"+
		"- name: test2\n"+
		"  uuid: test2-uuid\n"+
		"  ownertag: bob@local\n"+
		"  blocks:\n"+
		"  - BlockDestroy\n"+
		"  - BlockChange\n")
}
