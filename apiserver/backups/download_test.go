// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

func (s *backupsSuite) TestDownloadDirectOkay(c *gc.C) {
	impl := s.setBackups(c, s.meta, "")
	buf := bytes.NewBufferString("spamspamspam")
	expected := buf.Bytes()
	impl.archive = ioutil.NopCloser(buf)
	args := params.BackupsDownloadArgs{
		ID: "some-id",
	}
	result, err := s.api.DownloadDirect(args)
	c.Assert(err, gc.IsNil)

	c.Check(string(result.Data), gc.Equals, string(expected))
}

func (s *backupsSuite) TestDownloadDirectMissingFile(c *gc.C) {
	s.setBackups(c, s.meta, "")
	args := params.BackupsDownloadArgs{
		ID: "some-id",
	}
	_, err := s.api.DownloadDirect(args)

	c.Check(err, gc.ErrorMatches, `backup for "some-id" missing archive`)
}

func (s *backupsSuite) TestDownloadDirectError(c *gc.C) {
	s.setBackups(c, nil, "failed!")
	args := params.BackupsDownloadArgs{
		ID: "some-id",
	}
	_, err := s.api.DownloadDirect(args)

	c.Check(err, gc.ErrorMatches, "failed!")
}
