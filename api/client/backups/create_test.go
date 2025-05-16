// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	backupstesting "github.com/juju/juju/core/backups/testing"
	"github.com/juju/juju/rpc/params"
)

type createSuite struct {
	baseSuite
}

func TestCreateSuite(t *stdtesting.T) { tc.Run(t, &createSuite{}) }
func (s *createSuite) TestCreate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.BackupsCreateArgs{
		Notes:      "important",
		NoDownload: true,
	}
	meta := backupstesting.NewMetadata()
	result := params.CreateResult(meta, "test-filename")
	result.Notes = arg.Notes

	s.facade.EXPECT().FacadeCall(gomock.Any(), "Create", arg, gomock.Any()).SetArg(3, result)

	client := s.newClient()
	got, err := client.Create(c.Context(), "important", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Log(got)
	resultMeta := backupstesting.UpdateNotes(meta, "important")
	s.checkMetadataResult(c, got, resultMeta)
}
