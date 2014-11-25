// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
)

type uploadSuite struct {
	baseSuite
}

var _ = gc.Suite(&uploadSuite{})

func (s *uploadSuite) TestUploadFake(c *gc.C) {
	var sshHost, sshFilename string
	s.PatchValue(backups.TestSSHUpload, func(host, filename string, archive io.Reader) error {
		sshHost = host
		sshFilename = filename
		return nil
	})

	original := []byte("<compressed>")
	archive := bytes.NewBuffer(original)
	id, err := s.client.Upload(archive)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(sshHost, gc.Equals, "ubuntu@localhost")
	c.Check(sshFilename, gc.Matches, `juju-backup-.*\.tar.gz$`)
	c.Check(id, gc.Matches, `file://juju-backup-.*\.tar.gz$`)
}
