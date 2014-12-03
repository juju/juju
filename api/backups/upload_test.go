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
}

func (s *uploadSuite) TestFunctional(c *gc.C) {
	data := "<compressed archive data>"
	archive := ioutil.NopCloser(bytes.NewBufferString(data))

	var meta params.BackupsMetadataResult
	meta.UpdateFromMetadata(s.Meta)
	meta.ID = ""
	meta.Stored = time.Time{}

	id, err := s.client.Upload(archive, meta)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Matches, `[-\d]+\.[-0-9a-f]+`)
}
