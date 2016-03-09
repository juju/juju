// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ListBlocksSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api      *fakeListBlocksAPI
	apierror error
	store    *jujuclienttesting.MemStore
}

var _ = gc.Suite(&ListBlocksSuite{})

// fakeListBlocksAPI mocks out the controller API
type fakeListBlocksAPI struct {
	err    error
	blocks []params.ModelBlockInfo
}

func (f *fakeListBlocksAPI) Close() error { return nil }

func (f *fakeListBlocksAPI) ListBlockedModels() ([]params.ModelBlockInfo, error) {
	return f.blocks, f.err
}

func (s *ListBlocksSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.apierror = nil
	s.api = &fakeListBlocksAPI{
		blocks: []params.ModelBlockInfo{
			params.ModelBlockInfo{
				Name:     "test1",
				UUID:     "test1-uuid",
				OwnerTag: "cheryl@local",
				Blocks: []string{
					"BlockDestroy",
				},
			},
			params.ModelBlockInfo{
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
	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["dummysys"] = jujuclient.ControllerDetails{}
}

func (s *ListBlocksSuite) runListBlocksCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := controller.NewListBlocksCommandForTest(s.api, s.apierror, s.store)
	args = append(args, []string{"-c", "dummysys"}...)
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
	c.Check(testLog, jc.Contains, "Unable to list blocked models: unexpected api error")
}

func (s *ListBlocksSuite) TestListBlocksTabular(c *gc.C) {
	ctx, err := s.runListBlocksCommand(c)
	c.Check(err, jc.ErrorIsNil)
	c.Check(testing.Stdout(ctx), gc.Equals, ""+
		"NAME   MODEL UUID  OWNER         BLOCKS\n"+
		"test1  test1-uuid  cheryl@local  destroy-model\n"+
		"test2  test2-uuid  bob@local     destroy-model,all-changes\n"+
		"\n")
}

func (s *ListBlocksSuite) TestListBlocksJSON(c *gc.C) {
	ctx, err := s.runListBlocksCommand(c, "--format", "json")
	c.Check(err, jc.ErrorIsNil)
	c.Check(testing.Stdout(ctx), gc.Equals, "["+
		`{"name":"test1","model-uuid":"test1-uuid","owner-tag":"cheryl@local",`+
		`"blocks":["BlockDestroy"]},`+
		`{"name":"test2","model-uuid":"test2-uuid","owner-tag":"bob@local",`+
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
