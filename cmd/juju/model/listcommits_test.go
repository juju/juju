// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"
	"regexp"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type commitsSuite struct {
	generationBaseSuite

	api *mocks.MockCommitsCommandAPI
}

var _ = gc.Suite(&commitsSuite{})

func (s *commitsSuite) TestInitNoArg(c *gc.C) {
	err := s.runInit()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *commitsSuite) TestInitOneArg(c *gc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, gc.ErrorMatches, `expected no arguments, but got 1`)
}
func (s *commitsSuite) getMockValues() []coremodel.GenerationCommit {
	values := []coremodel.GenerationCommit{
		{
			Completed:    "0001-01-01",
			CompletedBy:  "test-user",
			GenerationId: 1,
			BranchName:   "bla",
		},
		{
			Completed:    "0001-02-02",
			CompletedBy:  "test-user",
			GenerationId: 2,
			BranchName:   "test",
		},
	}
	return values
}

func (s *commitsSuite) TestRunCommandTabularOutput(c *gc.C) {
	defer s.setup(c).Finish()
	result := s.getMockValues()
	expected :=
		`Commit	Committed at	Committed by	Branch name
1     	0001-01-01  	test-user   	bla        
2     	0001-02-02  	test-user   	test       
`
	s.api.EXPECT().ListCommits(gomock.Any()).Return(result, nil)

	ctx, err := s.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected)
}

func (s *commitsSuite) TestRunCommandJsonOutput(c *gc.C) {
	defer s.setup(c).Finish()
	result := s.getMockValues()
	unwrap := regexp.MustCompile(`[\s+\n]`)
	expected := unwrap.ReplaceAllLiteralString(`
{
  "commits": [
    {
      "id": 1,
      "branch-name": "bla",
      "committed-at": "0001-01-01",
      "committed-by": "test-user"
    },
    {
      "id": 2,
      "branch-name": "test",
      "committed-at": "0001-02-02",
      "committed-by": "test-user"
    }
  ]
}
`, "")
	expected = expected + "\n"
	s.api.EXPECT().ListCommits(gomock.Any()).Return(result, nil)

	ctx, err := s.runCommand(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected)
}

func (s *commitsSuite) TestRunCommandYamlOutput(c *gc.C) {
	defer s.setup(c).Finish()
	result := s.getMockValues()
	expected := `
commits:
- id: 1
  branch-name: bla
  committed-at: "0001-01-01"
  committed-by: test-user
- id: 2
  branch-name: test
  committed-at: "0001-02-02"
  committed-by: test-user
`[1:]
	s.api.EXPECT().ListCommits(gomock.Any()).Return(result, nil)

	ctx, err := s.runCommand(c, "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, expected)
}

func (s *commitsSuite) TestRunCommandAPIError(c *gc.C) {
	defer s.setup(c).Finish()

	s.api.EXPECT().ListCommits(gomock.Any()).Return(nil, errors.New("boom"))

	_, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *commitsSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewListCommitsCommandForTest(nil, s.store), args)
}

func (s *commitsSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewListCommitsCommandForTest(s.api, s.store), args...)
}

func (s *commitsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockCommitsCommandAPI(ctrl)
	s.api.EXPECT().Close()
	return ctrl
}
