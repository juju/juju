// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiserverbackups "github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/params"
)

type uploadSuite struct {
	httpSuite
}

var _ = gc.Suite(&uploadSuite{})

func (s *uploadSuite) setSuccess(c *gc.C, id string) {
	result := params.BackupsUploadResult{ID: id}
	s.setJSONSuccess(c, &result)
}

func (s *uploadSuite) TestSuccess(c *gc.C) {
	s.setSuccess(c, "<a new backup ID>")

	data := "<compressed archive data>"
	archive := ioutil.NopCloser(bytes.NewBufferString(data))

	meta := apiserverbackups.ResultFromMetadata(s.Meta)
	meta.ID = ""
	meta.Stored = time.Time{}

	id, err := s.client.Upload(archive, meta)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Equals, "<a new backup ID>")
	s.FakeClient.CheckCalledReader(c, "backups", archive, &meta, "juju-backup.tar.gz", "SendHTTPRequestReader")
}

func (s *uploadSuite) TestFunctional(c *gc.C) {
	data := "<compressed archive data>"
	archive := ioutil.NopCloser(bytes.NewBufferString(data))

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
	defer archive.Close()
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
