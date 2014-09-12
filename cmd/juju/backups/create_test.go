// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/backups"
)

const createExpectedHelp = `
usage: juju backups create [options] [<notes>]
purpose: create a backup

options:
-e, --environment (= "")
    juju environment to operate in
--quiet  (= false)
    do not print the metadata

"create" requests that juju create a backup of its state and print the
backup's unique ID.  You may provide a note to associate with the backup.

The backup archive and associated metadata are stored in juju and
will be lost when the environment is destroyed.
`

type createSuite struct {
	BackupsSuite
	Error   string
	command *backups.BackupsCreateCommand
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) SetUpTest(c *gc.C) {
	s.BackupsSuite.SetUpTest(c)

	s.PatchValue(
		backups.SendCreateRequest,
		func(cmd *backups.BackupsCreateCommand) (*params.BackupsMetadataResult, error) {
			if s.Error != "" {
				return nil, errors.New(s.Error)
			}
			return s.metaresult, nil
		},
	)

	s.Error = ""
	s.command = &backups.BackupsCreateCommand{}
}

func (s *createSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, "create", createExpectedHelp[1:])
}

func (s *createSuite) TestOkay(c *gc.C) {
	ctx := cmdtesting.Context(c)
	err := s.command.Run(ctx)
	c.Check(err, gc.IsNil)

	out := MetaResultString + s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, "")
}

func (s *createSuite) TestQuiet(c *gc.C) {
	s.command.Quiet = true
	ctx := cmdtesting.Context(c)
	err := s.command.Run(ctx)
	c.Check(err, gc.IsNil)

	out := s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, "")
}

func (s *createSuite) TestError(c *gc.C) {
	s.Error = "failed!"
	ctx := cmdtesting.Context(c)
	err := s.command.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
