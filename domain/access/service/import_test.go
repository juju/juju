// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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

// TestImportModelUsers verifies the target-wins resolution: an active target
// user is left alone, an external removed user is created then disabled, and a
// missing local user is reported inactive.
func (s *importServiceSuite) TestImportModelUsers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	everyoneUUID := usertesting.GenUserUUID(c)
	aliceName := usertesting.GenNewName(c, "alice")
	bobName, err := user.NewName("bob@external")
	c.Assert(err, tc.ErrorIsNil)
	carolName := usertesting.GenNewName(c, "carol")
	bobDate := time.Now().Add(-time.Hour).UTC()

	// alice is an active target user (found) -> left alone.
	s.state.EXPECT().GetUserByName(gomock.Any(), aliceName).Return(user.User{Name: aliceName}, nil)
	// bob@external is missing -> external+removed -> created then disabled.
	s.state.EXPECT().GetUserByName(gomock.Any(), bobName).Return(user.User{}, accesserrors.UserNotFound)
	// carol is missing and local -> inactive, never created.
	s.state.EXPECT().GetUserByName(gomock.Any(), carolName).Return(user.User{}, accesserrors.UserNotFound)

	s.state.EXPECT().GetUserUUIDByName(gomock.Any(), permission.EveryoneUserName).Return(everyoneUUID, nil)
	s.state.EXPECT().AddUserWithCreatedAt(
		gomock.Any(), gomock.Any(), bobName, "Bob", everyoneUUID, bobDate,
	).Return(nil)
	s.state.EXPECT().RemoveUser(gomock.Any(), bobName).Return(nil)

	inactive, err := s.service().ImportModelUsers(c.Context(), []coremodelmigration.ModelUser{{
		Name:     "alice",
		External: true,
	}, {
		Name:        "bob@external",
		DisplayName: "Bob",
		External:    true,
		Removed:     true,
		CreatedAt:   bobDate,
	}, {
		Name: "carol",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inactive.SortedValues(), tc.DeepEquals, []string{"bob@external", "carol"})
}

func (s *importServiceSuite) TestImportModelUsersEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	inactive, err := s.service().ImportModelUsers(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inactive.IsEmpty(), tc.IsTrue)
}

// TestImportModelPermissions verifies model grants are written individually,
// offer grants grouped, and inactive users skipped.
func (s *importServiceSuite) TestImportModelPermissions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must0(c, coremodel.NewUUID)
	offerUUID := uuid.MustNewUUID()

	s.state.EXPECT().CreatePermission(
		gomock.Any(), gomock.AssignableToTypeOf(uuid.UUID{}), gomock.AssignableToTypeOf(permission.UserAccessSpec{}),
	).Return(permission.UserAccess{}, nil)
	s.state.EXPECT().ImportOfferAccess(gomock.Any(), []access.OfferImportAccess{{
		UUID:   offerUUID,
		Access: map[string]permission.Access{"bob": permission.ConsumeAccess},
	}}).Return(nil)

	offerUUIDs, err := s.service().ImportModelPermissions(c.Context(), []coremodelmigration.ModelPermission{{
		SubjectName: "alice",
		ObjectType:  string(permission.Model),
		Access:      string(permission.AdminAccess),
		GrantOn:     modelUUID.String(),
	}, {
		SubjectName: "bob",
		ObjectType:  string(permission.Offer),
		Access:      string(permission.ConsumeAccess),
		GrantOn:     offerUUID.String(),
	}, {
		SubjectName: "inactiveuser",
		ObjectType:  string(permission.Model),
		Access:      string(permission.ReadAccess),
		GrantOn:     modelUUID.String(),
	}}, set.NewStrings("inactiveuser"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(offerUUIDs, tc.DeepEquals, []string{offerUUID.String()})
}

func (s *importServiceSuite) TestImportModelPermissionsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUIDs, err := s.service().ImportModelPermissions(c.Context(), nil, set.NewStrings())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(offerUUIDs, tc.HasLen, 0)
}

// TestImportLastModelLogins verifies last-login times are set for active users
// who logged in, and skipped for inactive users or those who never logged in.
func (s *importServiceSuite) TestImportLastModelLogins(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must0(c, coremodel.NewUUID)
	aliceName := usertesting.GenNewName(c, "alice")
	aliceLogin := time.Now().Add(-time.Hour).UTC()
	inactiveLogin := time.Now().UTC()

	s.state.EXPECT().UpdateLastModelLogin(gomock.Any(), aliceName, modelUUID, aliceLogin).Return(nil)

	err := s.service().ImportLastModelLogins(c.Context(), modelUUID, []coremodelmigration.ModelUser{{
		Name:      "alice",
		LastLogin: &aliceLogin,
	}, {
		Name: "bob",
	}, {
		Name:      "inactiveuser",
		LastLogin: &inactiveLogin,
	}}, set.NewStrings("inactiveuser"))
	c.Assert(err, tc.ErrorIsNil)
}
