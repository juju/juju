// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiserverbackups "github.com/juju/juju/apiserver/facades/client/backups"
	"github.com/juju/juju/rpc/params"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

type createSuite struct {
	baseSuite
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) TestCreate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.BackupsCreateArgs{
		Notes:      "important",
		NoDownload: true,
	}
	meta := backupstesting.NewMetadata()
	result := apiserverbackups.CreateResult(meta, "test-filename")
	result.Notes = arg.Notes

	s.facade.EXPECT().FacadeCall("Create", arg, gomock.Any()).SetArg(2, result)

	client := s.newClient()
	got, err := client.Create("important", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Log(got)
	resultMeta := backupstesting.UpdateNotes(meta, "important")
	s.checkMetadataResult(c, got, resultMeta)
}
