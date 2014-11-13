// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"os"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
)

type uploadSuite struct {
	baseSuite
}

var _ = gc.Suite(&uploadSuite{})

func (s *uploadSuite) TestUpload(c *gc.C) {
	cleanup := backups.PatchBaseFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Assert(req, gc.Equals, "PublicAddress")

			if result, ok := resp.(*params.PublicAddressResults); ok {
				result.PublicAddress = "127.0.0.1"
			} else {
				c.Fatalf("wrong output structure")
			}
			return nil
		},
	)
	defer cleanup()

	var sshFilename string
	s.PatchValue(backups.TestSSHCopy, func(filename, remote string) error {
		sshFilename = filename
		c.Check(filename, jc.HasPrefix, "/tmp/juju-backup-")
		c.Check(remote, gc.Equals, "ubuntu@127.0.0.1:"+filename)
		return nil
	})

	original := []byte("<compressed>")
	archive := bytes.NewBuffer(original)

	id, err := s.client.Upload(archive)
	c.Assert(err, gc.IsNil)
	c.Check(id, gc.Equals, "file://"+sshFilename)

	filename := strings.TrimPrefix(id, "file://")
	c.Assert(filename, gc.Equals, sshFilename)
	_, err = os.Stat(filename)
	c.Assert(errors.Cause(err), jc.Satisfies, os.IsNotExist)
}
