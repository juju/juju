// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type showCommitsSuite struct {
	generationBaseSuite

	api *mocks.MockShowCommitCommandAPI
}

var _ = gc.Suite(&showCommitsSuite{})

func (s *showCommitsSuite) TestInitNoArg(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, "expected exactly 1 commit id, got 0 arguments")
}

func (s *showCommitsSuite) TestInitOneArg(c *gc.C) {
	err := s.runInit("1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *showCommitsSuite) TestInitNotInt(c *gc.C) {
	err := s.runInit("something")
	c.Assert(err, gc.ErrorMatches, `encountered problem trying to parse "something" into an int`)
}

func (s *showCommitsSuite) TestInitMoreArgs(c *gc.C) {
	args := []string{"1", "2", "3"}
	err := s.runInit(args...)
	c.Assert(err, gc.ErrorMatches, "expected exactly 1 commit id, got 3 arguments")
}
func (s *showCommitsSuite) getGenerationCommitValue() coremodel.GenerationCommit {
	values := coremodel.GenerationCommit{
		Completed:    time.Unix(12345, 0),
		CompletedBy:  "test-user",
		Created:      time.Unix(12345, 0),
		CreatedBy:    "test-user",
		GenerationId: 1,
		BranchName:   "bla",
		Applications: []coremodel.GenerationCommitApplication{{
			ApplicationName: "redis",
			UnitsTracking:   []string{"redis/0"},
			ConfigChanges:   map[string]interface{}{"databases": 8},
		}},
	}
	return values
}

func (s *showCommitsSuite) TestYamlOutput(c *gc.C) {
	defer s.setup(c).Finish()
	result := s.getGenerationCommitValue()
	expected := `
branch:
  bla:
    applications:
    - application: redis
      units:
      - redis/0
      config:
        databases: 8
committed-at: 01 Jan 1970 04:25:45+01:00
committed-by: test-user
created: 01 Jan 1970 04:25:45+01:00
created-by: test-user
`[1:]
	s.api.EXPECT().ShowCommit(1).Return(result, nil)
	ctx, err := s.runCommand(c, "1", "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected)
}

func (s *showCommitsSuite) TestRunCommandAPIError(c *gc.C) {
	defer s.setup(c).Finish()

	s.api.EXPECT().ShowCommit(gomock.Any()).Return(coremodel.GenerationCommit{}, errors.New("boom"))

	_, err := s.runCommand(c, "1")
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *showCommitsSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewShowCommitCommandForTest(nil, s.store), args)
}

func (s *showCommitsSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewShowCommitCommandForTest(s.api, s.store), args...)
}

func (s *showCommitsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockShowCommitCommandAPI(ctrl)
	s.api.EXPECT().Close()
	return ctrl
}
