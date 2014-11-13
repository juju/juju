// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
)

type uploadSuite struct {
	baseSuite
}

var _ = gc.Suite(&uploadSuite{})

func (s *uploadSuite) TestUploadFake(c *gc.C) {
	s.PatchValue(backups.TestSSHUpload, func(addr string, archive io.Reader) (string, error) {
		return "file://juju-backup-20141111-010203.tgz", nil
	})

	original := []byte("<compressed>")
	archive := bytes.NewBuffer(original)
	id, err := s.client.Upload(archive)
	c.Assert(err, gc.IsNil)

	c.Check(id, gc.Equals, "file://juju-backup-20141111-010203.tgz")
}
