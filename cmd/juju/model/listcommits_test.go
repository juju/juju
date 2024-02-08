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

type commitsSuite struct {
	generationBaseSuite

	api *mocks.MockCommitsCommandAPI
}

var _ = gc.Suite(&commitsSuite{})

func (cs *commitsSuite) TestInitNoArg(c *gc.C) {
	err := cs.runInit()
	c.Assert(err, jc.ErrorIsNil)
}

func (cs *commitsSuite) TestInitOneArg(c *gc.C) {
	err := cs.runInit(cs.branchName)
	c.Assert(err, gc.ErrorMatches, `expected no arguments, but got 1`)
}
func (cs *commitsSuite) getGenerationCommitValues() []coremodel.GenerationCommit {
	values := []coremodel.GenerationCommit{
		{
			Completed:    time.Unix(12345, 0),
			CompletedBy:  "test-user",
			GenerationId: 1,
			BranchName:   "bla",
		},
		{
			Completed:    time.Unix(12345, 0),
			CompletedBy:  "test-user",
			GenerationId: 2,
			BranchName:   "test",
		},
	}
	return values
}

func (cs *commitsSuite) TestRunCommandTabularOutput(c *gc.C) {
	defer cs.setup(c).Finish()
	result := cs.getGenerationCommitValues()
	expected := `
Commit	Committed at	Committed by	Branch name
2     	1970-01-01  	test-user   	test       
1     	1970-01-01  	test-user   	bla        
`[1:]
	cs.api.EXPECT().ListCommits().Return(result, nil)

	ctx, err := cs.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expected)
}

func (cs *commitsSuite) TestRunCommandAPIError(c *gc.C) {
	defer cs.setup(c).Finish()

	cs.api.EXPECT().ListCommits().Return(nil, errors.New("boom"))

	_, err := cs.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (cs *commitsSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewListCommitsCommandForTest(nil, cs.store), args)
}

func (cs *commitsSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewListCommitsCommandForTest(cs.api, cs.store), args...)
}

func (cs *commitsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	cs.api = mocks.NewMockCommitsCommandAPI(ctrl)
	cs.api.EXPECT().Close()
	return ctrl
}
