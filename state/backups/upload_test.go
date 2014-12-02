// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/testing"
)

type uploadSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&uploadSuite{})

func (s *uploadSuite) TestSSHUpload(c *gc.C) {
	var sshFilename, sshHost string
	var sshArchive io.Reader
	fakeUpload := func(host, filename string, archive io.Reader) error {
		sshHost = host
		sshFilename = filename
		sshArchive = archive
		return nil
	}

	original := []byte("<compressed>")
	archive := bytes.NewBuffer(original)

	id, err := backups.SimpleUpload("127.0.0.1", archive, fakeUpload)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(sshFilename, gc.Matches, `juju-backup-.*\.tar\.gz$`)
	c.Check(sshHost, gc.Equals, "ubuntu@127.0.0.1")
	c.Check(sshArchive, gc.Equals, archive)
	c.Check(id, gc.Equals, "file://"+sshFilename)
}
