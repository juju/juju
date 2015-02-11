// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/params"
)

func (s *backupsSuite) TestInfoOkay(c *gc.C) {
	impl := s.setBackups(c, s.meta, "")
	impl.Archive = ioutil.NopCloser(bytes.NewBufferString("spamspamspam"))
	args := params.BackupsInfoArgs{
		ID: "some-id",
	}
	result, err := s.api.Info(args)
	c.Assert(err, jc.ErrorIsNil)
	expected := backups.ResultFromMetadata(s.meta)
	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestInfoMissingFile(c *gc.C) {
	s.setBackups(c, s.meta, "")
	args := params.BackupsInfoArgs{
		ID: "some-id",
	}
	result, err := s.api.Info(args)
	c.Assert(err, jc.ErrorIsNil)
	expected := backups.ResultFromMetadata(s.meta)

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestInfoError(c *gc.C) {
	s.setBackups(c, nil, "failed!")
	args := params.BackupsInfoArgs{
		ID: "some-id",
	}
	_, err := s.api.Info(args)

	c.Check(err, gc.ErrorMatches, "failed!")
}
