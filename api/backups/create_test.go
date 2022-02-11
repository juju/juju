// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	apiserverbackups "github.com/juju/juju/apiserver/facades/client/backups"
	"github.com/juju/juju/rpc/params"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

type createSuite struct {
	baseSuite
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) TestCreate(c *gc.C) {
	cleanup := backups.PatchClientFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Check(req, gc.Equals, "Create")

			c.Assert(paramsIn, gc.FitsTypeOf, params.BackupsCreateArgs{})
			p := paramsIn.(params.BackupsCreateArgs)
			c.Check(p.Notes, gc.Equals, "important")
			c.Check(p.KeepCopy, jc.IsFalse)
			c.Check(p.NoDownload, jc.IsFalse)

			if result, ok := resp.(*params.BackupsMetadataResult); ok {
				*result = apiserverbackups.CreateResult(s.Meta, "test-filename")
				result.Notes = p.Notes
			} else {
				c.Fatalf("wrong output structure")
			}
			return nil
		},
	)
	defer cleanup()

	result, err := s.client.Create("important", false, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Log(result)
	meta := backupstesting.UpdateNotes(s.Meta, "important")
	s.checkMetadataResult(c, result, meta)
}
