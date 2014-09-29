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

func (s *uploadSuite) TestUpload(c *gc.C) {
	data := "<compressed archive data>"
	archive := ioutil.NopCloser(bytes.NewBufferString(data))

	var meta params.BackupsMetadataResult
	meta.UpdateFromMetadata(s.Meta)
	meta.ID = ""
	meta.Stored = false

	cleanup := backups.PatchClientFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Check(req, gc.Equals, "UploadDirect")

			c.Assert(paramsIn, gc.FitsTypeOf, params.BackupsUploadArgs{})
			p := paramsIn.(params.BackupsUploadArgs)
			c.Check(string(p.Data), gc.Equals, data)
			c.Check(p.Metadata, gc.Equals, meta)

			if result, ok := resp.(*params.BackupsMetadataResult); ok {
				result.UpdateFromMetadata(s.Meta)
			} else {
				c.Fatalf("wrong output structure")
			}
			return nil
		},
	)
	defer cleanup()

	result, err := s.client.Upload(archive, meta)
	c.Assert(err, gc.IsNil)

	s.checkMetadataResult(c, result, s.Meta)
}
