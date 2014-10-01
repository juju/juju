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

func (s *uploadSuite) TestUpload(c *gc.C) {
	s.setSuccess(c, "<a new backup ID>")

	data := "<compressed archive data>"
	archive := ioutil.NopCloser(bytes.NewBufferString(data))

	meta := apiserverbackups.ResultFromMetadata(s.Meta)
	meta.ID = ""
	meta.Stored = time.Time{}

	result, err := s.client.Upload(archive, meta)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.ID, gc.Equals, "<a new backup ID>")
}
