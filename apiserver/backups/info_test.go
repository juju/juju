// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
)

func (s *backupsSuite) TestInfoOkay(c *gc.C) {
	impl := s.setBackups(c, s.meta, "")
	impl.archive = ioutil.NopCloser(bytes.NewBufferString("spamspamspam"))
	args := params.BackupsInfoArgs{
		ID: "some-id",
	}
	result, err := s.api.Info(args)
	c.Assert(err, gc.IsNil)
	expected := params.BackupsMetadataResult{}
	expected.UpdateFromMetadata(s.meta)

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestInfoMissingFile(c *gc.C) {
	s.setBackups(c, s.meta, "")
	args := params.BackupsInfoArgs{
		ID: "some-id",
	}
	result, err := s.api.Info(args)
	c.Assert(err, gc.IsNil)
	expected := params.BackupsMetadataResult{}
	expected.UpdateFromMetadata(s.meta)

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
