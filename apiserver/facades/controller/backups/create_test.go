// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type backupsSuite struct {
	authorizer     *MockAuthorizer
	controllerUUID string
}

func TestBackupsSuite(t *testing.T) { tc.Run(t, &backupsSuite{}) }
func (s *backupsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.authorizer = NewMockAuthorizer(ctrl)
	s.controllerUUID = uuid.MustNewUUID().String()
	return ctrl
}

// newAPI returns a backups API wired to the suite's authorizer mock.
func (s *backupsSuite) newAPI(c *tc.C) *API {
	api, err := NewAPI(
		s.authorizer,
		s.controllerUUID,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

// expectSuperuser primes the authorizer mock so NewAPI and Create accept
// the client, with the superuser check targeting the controller tag.
func (s *backupsSuite) expectSuperuser() {
	s.authorizer.EXPECT().AuthClient().Return(true).AnyTimes()
	s.authorizer.EXPECT().HasPermission(
		gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID),
	).Return(nil).AnyTimes()
}

// TestNewAPINotClient verifies that NewAPI rejects non-client callers.
func (s *backupsSuite) TestNewAPINotClient(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthClient().Return(false)

	api, err := NewAPI(
		s.authorizer,
		s.controllerUUID,
		loggertesting.WrapCheckLog(c),
	)
	c.Check(api, tc.IsNil)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestCreateRejected verifies that the RPC Create method rejects every
// call: backup creation moved to the backups HTTP endpoint, so older
// clients get a clear, classified upgrade error before any work runs.
func (s *backupsSuite) TestCreateRejected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectSuperuser()

	_, err := s.newAPI(c).Create(c.Context(), params.BackupsCreateArgs{Notes: "test"})
	c.Assert(err, tc.ErrorMatches,
		"create-backup from clients older than 4.1 is not supported; use a 4.1 or newer client")
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *backupsSuite) TestCreateNotSuperuser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthClient().Return(true).AnyTimes()
	s.authorizer.EXPECT().HasPermission(
		gomock.Any(), permission.SuperuserAccess, gomock.Any(),
	).Return(coreerrors.Forbidden)

	_, err := s.newAPI(c).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, coreerrors.Forbidden)
}
