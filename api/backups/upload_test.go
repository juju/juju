// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io/ioutil"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiserverbackups "github.com/juju/juju/apiserver/backups"
)

type uploadSuite struct {
	baseSuite
}

var _ = gc.Suite(&uploadSuite{})

func (s *uploadSuite) TestSuccessfulRequest(c *gc.C) {
	data := "<compressed archive data>"
	archive := strings.NewReader(data)

	meta := apiserverbackups.ResultFromMetadata(s.Meta)
	meta.ID = ""
	meta.Stored = time.Time{}
	meta.Size = int64(len(data))

	id, err := s.client.Upload(archive, meta)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Matches, `[-\d]+\.[-0-9a-f]+`)

	// Check the stored contents.
	stored, err := s.client.Download(id)
	c.Assert(err, jc.ErrorIsNil)
	storedData, err := ioutil.ReadAll(stored)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(storedData), gc.Equals, data)

	// Check the stored metadata.
	storedMeta, err := s.client.Info(id)
	c.Assert(err, jc.ErrorIsNil)
	meta.ID = id
	meta.Stored = storedMeta.Stored
	c.Check(storedMeta, gc.DeepEquals, &meta)
}

func (s *uploadSuite) TestFailedRequest(c *gc.C) {
	data := "<compressed archive data>"
	archive := strings.NewReader(data)

	meta := apiserverbackups.ResultFromMetadata(s.Meta)
	meta.ID = ""
	meta.Size = int64(len(data))
	// The Model field is required, so zero it so that
	// we'll get an error from the endpoint.
	meta.Model = ""

	id, err := s.client.Upload(archive, meta)
	c.Assert(err, gc.ErrorMatches, `PUT https://.*/model/.*/backups: while storing backup archive: missing Model`)
	c.Assert(id, gc.Equals, "")
}
