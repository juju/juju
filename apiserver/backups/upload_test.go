// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

func (s *backupsSuite) metaArg() params.BackupsMetadataResult {
	s.meta.Finish(10, "<checksum>", "", nil)

	var meta params.BackupsMetadataResult
	meta.UpdateFromMetadata(s.meta)
	meta.ID = ""
	return meta
}

func (s *backupsSuite) TestUploadDirectOkay(c *gc.C) {
	s.setBackups(c, s.meta, "")

	data := []byte("spamspamspam")
	args := params.BackupsUploadArgs{
		Data:     data,
		Metadata: s.metaArg(),
	}
	result, err := s.api.UploadDirect(args)
	c.Assert(err, gc.IsNil)

	var expected params.BackupsMetadataResult
	expected.UpdateFromMetadata(s.meta)
	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestUploadDirectError(c *gc.C) {
	s.setBackups(c, nil, "failed!")

	args := params.BackupsUploadArgs{
		Data:     []byte{},
		Metadata: s.metaArg(),
	}
	_, err := s.api.UploadDirect(args)

	c.Check(err, gc.ErrorMatches, "failed!")
}
