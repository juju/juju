// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/internal"
	"github.com/juju/juju/internal/errors"
)

type importServiceSuite struct {
	state *MockState
}

func TestImportServiceSuite(t *testing.T) {
	tc.Run(t, &importServiceSuite{})
}

func (s *importServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *importServiceSuite) service() *Service {
	return NewService(s.state, clock.WallClock)
}

// TestImportExternalUsersEmpty verifies that calling ImportExternalUsers with
// an empty slice is a no-op.
func (s *importServiceSuite) TestImportExternalUsersEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().ImportExternalUsers(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportExternalUsers verifies that external users are created with
// everyone@external as their creator and their original creation date.
func (s *importServiceSuite) TestImportExternalUsers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	everyoneUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	bobName, err := user.NewName("bob@external")
	c.Assert(err, tc.ErrorIsNil)
	aliceName, err := user.NewName("alice@external")
	c.Assert(err, tc.ErrorIsNil)

	bobDate := time.Now().Add(-2 * time.Hour).UTC()
	aliceDate := time.Now().Add(-time.Hour).UTC()

	users := []internal.ExternalUserImport{
		{Name: bobName, DisplayName: "Bob", DateCreated: bobDate},
		{Name: aliceName, DisplayName: "Alice", DateCreated: aliceDate},
	}

	s.state.EXPECT().
		GetUserUUIDByName(gomock.Any(), permission.EveryoneUserName).
		Return(everyoneUUID, nil)
	s.state.EXPECT().
		AddUserWithCreatedAt(
			gomock.Any(), gomock.Any(), bobName, "Bob", everyoneUUID, bobDate,
		).Return(nil)
	s.state.EXPECT().
		AddUserWithCreatedAt(
			gomock.Any(), gomock.Any(), aliceName, "Alice", everyoneUUID, aliceDate,
		).Return(nil)

	err = s.service().ImportExternalUsers(c.Context(), users)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportExternalUsersAlreadyExists verifies that a user that already exists
// on the target controller is silently skipped.
func (s *importServiceSuite) TestImportExternalUsersAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	everyoneUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	bobName, err := user.NewName("bob@external")
	c.Assert(err, tc.ErrorIsNil)

	users := []internal.ExternalUserImport{
		{Name: bobName, DisplayName: "Bob", DateCreated: time.Now()},
	}

	s.state.EXPECT().
		GetUserUUIDByName(gomock.Any(), permission.EveryoneUserName).
		Return(everyoneUUID, nil)
	s.state.EXPECT().
		AddUserWithCreatedAt(
			gomock.Any(), gomock.Any(), bobName, "Bob", everyoneUUID, gomock.Any(),
		).Return(accesserrors.UserAlreadyExists)

	err = s.service().ImportExternalUsers(c.Context(), users)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportExternalUsersEveryoneNotFound verifies that an error is returned
// when everyone@external does not exist on the target controller.
func (s *importServiceSuite) TestImportExternalUsersEveryoneNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bobName, err := user.NewName("bob@external")
	c.Assert(err, tc.ErrorIsNil)

	users := []internal.ExternalUserImport{
		{Name: bobName, DisplayName: "Bob", DateCreated: time.Now()},
	}

	s.state.EXPECT().
		GetUserUUIDByName(gomock.Any(), permission.EveryoneUserName).
		Return(user.UUID(""), accesserrors.UserNotFound)

	err = s.service().ImportExternalUsers(c.Context(), users)
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestImportExternalUsersStateError verifies that unexpected state errors during
// user creation are propagated to the caller.
func (s *importServiceSuite) TestImportExternalUsersStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	everyoneUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	bobName, err := user.NewName("bob@external")
	c.Assert(err, tc.ErrorIsNil)

	users := []internal.ExternalUserImport{
		{Name: bobName, DisplayName: "Bob", DateCreated: time.Now()},
	}

	expected := errors.New("boom")

	s.state.EXPECT().
		GetUserUUIDByName(gomock.Any(), permission.EveryoneUserName).
		Return(everyoneUUID, nil)
	s.state.EXPECT().
		AddUserWithCreatedAt(
			gomock.Any(), gomock.Any(), bobName, "Bob", everyoneUUID, gomock.Any(),
		).Return(expected)

	err = s.service().ImportExternalUsers(c.Context(), users)
	c.Assert(err, tc.ErrorIs, expected)
}
