// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
)

type uploadSuite struct {
	baseSuite
}

var _ = gc.Suite(&uploadSuite{})

func (s *uploadSuite) TestSuccess(c *gc.C) {
	result := params.BackupsUploadResult{ID: "<some backup ID>"}
	backups.SetHTTP(s.client, &fakeHTTPCaller{Result: result})

	data := "<compressed archive data>"
	archive := ioutil.NopCloser(bytes.NewBufferString(data))

	var meta params.BackupsMetadataResult
	meta.UpdateFromMetadata(s.Meta)
	meta.ID = ""
	meta.Stored = false

	id, err := s.client.Upload(archive, meta)
	c.Assert(err, gc.IsNil)

	c.Check(id, gc.Equals, "<some backup ID>")
}

func (s *uploadSuite) TestFunctional(c *gc.C) {
	//result := params.BackupsUploadResult{ID: "<some backup ID>"}
	//backups.SetHTTP(s.client, &fakeHTTPCaller{Result: result})

	data := "<compressed archive data>"
	archive := ioutil.NopCloser(bytes.NewBufferString(data))

	var meta params.BackupsMetadataResult
	meta.UpdateFromMetadata(s.Meta)
	meta.ID = ""
	meta.Stored = false

	id, err := s.client.Upload(archive, meta)
	c.Assert(err, gc.IsNil)

	c.Check(id, gc.Matches, `[-\d]+\.[-0-9a-f]+`)
}
