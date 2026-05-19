// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"reflect"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	backupstesting "github.com/juju/juju/core/backups/testing"
	"github.com/juju/juju/rpc/params"
)

type createSuite struct {
	baseSuite
}

func TestCreateSuite(t *testing.T) {
	tc.Run(t, &createSuite{})
}

func (s *createSuite) TestCreate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.BackupsCreateArgs{
		Notes:      "important",
		NoDownload: true,
	}
	meta := backupstesting.NewMetadata()
	result := params.CreateResult(meta, "test-filename")
	result.Notes = arg.Notes

	s.facade.EXPECT().FacadeCall(
		gomock.Any(), "Create", arg, gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, _ interface{}, resPtr interface{}) error {
		reflect.ValueOf(resPtr).Elem().Set(reflect.ValueOf(result))
		return nil
	})

	client := s.newClient()
	got, err := client.Create(c.Context(), "important", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Log(got)
	resultMeta := backupstesting.UpdateNotes(meta, "important")
	s.checkMetadataResult(c, got, resultMeta)
}
