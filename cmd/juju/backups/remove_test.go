// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/golang/mock/gomock"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/core/params"
)

type removeSuite struct {
	BaseBackupsSuite

	command cmd.Command
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)

	s.command = backups.NewRemoveCommandForTest(s.store)
}

func (s *removeSuite) patch(c *gc.C) (*gomock.Controller, *MockAPIClient) {
	ctrl := gomock.NewController(c)
	client := NewMockAPIClient(ctrl)
	s.PatchValue(backups.NewGetAPI,
		func(c *backups.CommandBase) (backups.APIClient, int, error) {
			return client, 2, nil
		},
	)
	return ctrl, client
}

func (s *removeSuite) TestRemovePassWithId(c *gc.C) {
	ctrl, client := s.patch(c)
	defer ctrl.Finish()

	gomock.InOrder(
		client.EXPECT().Remove([]string{"one"}).Return(
			[]params.ErrorResult{
				{},
			}, nil,
		),
		client.EXPECT().Close(),
	)
	ctx, err := cmdtesting.RunCommand(c, s.command, "one")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "successfully removed: one\n")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
}

var passWithKeepLatest = `
successfully removed: four
successfully removed: one
successfully removed: three
kept: two
`[1:]

func (s *removeSuite) TestRemovePassWithKeepLatest(c *gc.C) {
	ctrl, client := s.patch(c)
	defer ctrl.Finish()
	one := time.Now().Add(time.Minute * 20)
	two := time.Now().Add(time.Hour * 1)
	three := time.Now().Add(time.Minute * 40)
	four := time.Now()

	gomock.InOrder(
		client.EXPECT().List().Return(
			&params.BackupsListResult{
				List: []params.BackupsMetadataResult{
					{ID: "one", Started: one},
					{ID: "three", Started: three},
					{ID: "two", Started: two},
					{ID: "four", Started: four},
				},
			}, nil,
		),
		client.EXPECT().Remove([]string{"four", "one", "three"}).Return(
			[]params.ErrorResult{{}, {}, {}}, nil,
		),
		client.EXPECT().Close(),
	)
	ctx, err := cmdtesting.RunCommand(c, s.command, "--keep-latest")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, passWithKeepLatest)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
}

var failWithKeepLatest = `
successfully removed: three
successfully removed: two
kept: four
`[1:]

func (s *removeSuite) TestRemoveFailWithKeepLatest(c *gc.C) {
	ctrl, client := s.patch(c)
	defer ctrl.Finish()
	one := time.Now().Add(time.Minute * 20)
	two := time.Now()
	three := time.Now().Add(time.Minute * 40)
	four := time.Now().Add(time.Hour * 1)

	gomock.InOrder(
		client.EXPECT().List().Return(
			&params.BackupsListResult{
				List: []params.BackupsMetadataResult{
					{ID: "one", Started: one},
					{ID: "three", Started: three},
					{ID: "two", Started: two},
					{ID: "four", Started: four},
				},
			}, nil,
		),
		client.EXPECT().Remove([]string{"one", "three", "two"}).Return(
			[]params.ErrorResult{{Error: &params.Error{Message: "failme"}}, {}, {}},
			nil,
		),
		client.EXPECT().Close(),
	)
	ctx, err := cmdtesting.RunCommand(c, s.command, "--keep-latest")
	c.Check(errors.Cause(err), gc.ErrorMatches, "failed to remove one: failme")
	c.Assert(bufferString(ctx.Stderr), gc.Equals, failWithKeepLatest)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
}

func (s *removeSuite) TestRemoveFailWithId(c *gc.C) {
	ctrl, client := s.patch(c)
	defer ctrl.Finish()

	gomock.InOrder(
		client.EXPECT().Remove([]string{"spam"}).Return(
			[]params.ErrorResult{{Error: &params.Error{Message: "failed!"}}},
			nil,
		),
		client.EXPECT().Close(),
	)

	_, err := cmdtesting.RunCommand(c, s.command, "spam")
	c.Check(errors.Cause(err), gc.ErrorMatches, "failed to remove spam: failed!")
}

func (s *removeSuite) TestRemoveFail(c *gc.C) {
	ctrl, client := s.patch(c)
	defer ctrl.Finish()

	gomock.InOrder(
		client.EXPECT().Remove([]string{"spam"}).Return(
			nil,
			errors.Errorf("not found"),
		),
		client.EXPECT().Close(),
	)

	_, err := cmdtesting.RunCommand(c, s.command, "spam")
	c.Check(errors.Cause(err), gc.ErrorMatches, "not found")
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}
