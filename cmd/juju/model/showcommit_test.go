// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"errors"
	"time"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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
		Applications: []coremodel.GenerationApplication{{
			ApplicationName: "redis",
			UnitDetail: &coremodel.GenerationUnits{
				UnitsTracking: []string{"redis/0", "redis/1", "redis/2"}},
			ConfigChanges: map[string]interface{}{"databases": 8},
		}, {
			ApplicationName: "mysql",
			UnitDetail: &coremodel.GenerationUnits{
				UnitsTracking: []string{"mysql/0", "mysql/1", "mysql/2"}},
			ConfigChanges: map[string]interface{}{"connection": "localhost"},
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
        tracking:
        - redis/0
        - redis/1
        - redis/2
      config:
        databases: 8
    - application: mysql
      units:
        tracking:
        - mysql/0
        - mysql/1
        - mysql/2
      config:
        connection: localhost
committed-at: 1970-01-01 03:25:45Z
committed-by: test-user
created: 1970-01-01 03:25:45Z
created-by: test-user
`[1:]
	s.api.EXPECT().ShowCommit(1).Return(result, nil)
	ctx, err := s.runCommand(c, "1", "--format=yaml", "--utc")
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
