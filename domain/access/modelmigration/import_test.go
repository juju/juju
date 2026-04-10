// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service: s.service,
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestNoModelUserPermissions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	modelUUID := model.UUID()
	modelID := permission.ID{
		ObjectType: permission.Model,
		Key:        modelUUID,
	}
	bobName := usertesting.GenNewName(c, "bob")
	bobTime := time.Now().Truncate(time.Minute).UTC()
	bob := description.UserArgs{
		Name:           "bob",
		Access:         string(permission.AdminAccess),
		CreatedBy:      "creator",
		DateCreated:    time.Now(),
		DisplayName:    "bob",
		LastConnection: bobTime,
	}
	bazzaName := usertesting.GenNewName(c, "bazza")
	bazzaTime := time.Now().Truncate(time.Minute).UTC().Add(-time.Minute)
	bazza := description.UserArgs{
		Name:           "bazza",
		Access:         string(permission.ReadAccess),
		CreatedBy:      "bob",
		DateCreated:    time.Now(),
		DisplayName:    "bazza",
		LastConnection: bazzaTime,
	}

	model.AddUser(bob)
	model.AddUser(bazza)
	s.service.EXPECT().CreatePermission(gomock.Any(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: modelID,
			Access: permission.Access(bazza.Access),
		},
		User: bazzaName,
	})
	s.service.EXPECT().CreatePermission(gomock.Any(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: modelID,
			Access: permission.Access(bob.Access),
		},
		User: bobName,
	})
	s.service.EXPECT().SetLastModelLogin(gomock.Any(), bazzaName, coremodel.UUID(modelUUID), bazzaTime)
	s.service.EXPECT().SetLastModelLogin(gomock.Any(), bobName, coremodel.UUID(modelUUID), bobTime)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportPermissionAlreadyExists tests that permissions that already exist
// are ignored. This covers the permission of the model creator which is added
// the model is added.
func (s *importSuite) TestImportPermissionAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	modelUUID := model.UUID()
	modelID := permission.ID{
		ObjectType: permission.Model,
		Key:        modelUUID,
	}
	admin := description.UserArgs{
		Name:           "admin",
		Access:         string(permission.AdminAccess),
		CreatedBy:      "admin",
		DateCreated:    time.Now(),
		DisplayName:    "admin",
		LastConnection: time.Time{},
	}
	model.AddUser(admin)
	s.service.EXPECT().CreatePermission(gomock.Any(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: modelID,
			Access: permission.AdminAccess,
		},
		User: user.AdminUserName,
	}).Return(permission.UserAccess{}, accesserrors.PermissionAlreadyExists)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportPermissionUserDisabled tests that this error is returned to the
// user.
func (s *importSuite) TestImportPermissionUserDisabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	modelUUID := model.UUID()
	modelID := permission.ID{
		ObjectType: permission.Model,
		Key:        modelUUID,
	}
	disabledUser := description.UserArgs{
		Name:           "disabledUser",
		Access:         string(permission.AdminAccess),
		CreatedBy:      "disabledUser",
		DateCreated:    time.Now(),
		DisplayName:    "disabledUser",
		LastConnection: time.Time{},
	}
	model.AddUser(disabledUser)
	s.service.EXPECT().CreatePermission(gomock.Any(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: modelID,
			Access: permission.AdminAccess,
		},
		User: usertesting.GenNewName(c, "disabledUser"),
	}).Return(permission.UserAccess{}, accesserrors.UserAuthenticationDisabled)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, accesserrors.UserAuthenticationDisabled)
}

// TestImportExternalUsers verifies that external users are created via
// ImportExternalUsers and that permissions and last login are set in a single
// unified pass alongside local users.
func (s *importSuite) TestImportExternalUsers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	modelUUID := model.UUID()
	modelID := permission.ID{
		ObjectType: permission.Model,
		Key:        modelUUID,
	}

	adminTime := time.Now().Truncate(time.Minute).UTC()
	adminArgs := description.UserArgs{
		Name:           "admin",
		Access:         string(permission.AdminAccess),
		CreatedBy:      "admin",
		DateCreated:    time.Now(),
		DisplayName:    "admin",
		LastConnection: adminTime,
	}

	extTime := time.Now().Truncate(time.Minute).UTC()
	extArgs := description.UserArgs{
		Name:           "bob@external",
		Access:         string(permission.ReadAccess),
		CreatedBy:      "bob@external",
		DateCreated:    time.Now().Add(-time.Hour),
		DisplayName:    "Bob External",
		LastConnection: extTime,
	}

	model.AddUser(adminArgs)
	model.AddUser(extArgs)

	bobExtName, err := user.NewName("bob@external")
	c.Assert(err, tc.ErrorIsNil)

	// The permissions operation no longer calls ImportExternalUsers — that is
	// handled by the separate importExternalUsersOperation.
	s.service.EXPECT().CreatePermission(gomock.Any(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: modelID,
			Access: permission.AdminAccess,
		},
		User: user.AdminUserName,
	})
	s.service.EXPECT().CreatePermission(gomock.Any(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: modelID,
			Access: permission.ReadAccess,
		},
		User: bobExtName,
	})
	s.service.EXPECT().SetLastModelLogin(gomock.Any(), user.AdminUserName, coremodel.UUID(modelUUID), adminTime)
	s.service.EXPECT().SetLastModelLogin(gomock.Any(), bobExtName, coremodel.UUID(modelUUID), extTime)

	op := s.newImportOperation()
	err = op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

type importExternalUsersSuite struct {
	coordinator *MockCoordinator
	service     *MockImportExternalUsersService
}

func TestImportExternalUsersSuite(t *testing.T) {
	tc.Run(t, &importExternalUsersSuite{})
}

func (s *importExternalUsersSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportExternalUsersService(ctrl)

	c.Cleanup(func() {
		s.coordinator = nil
		s.service = nil
	})

	return ctrl
}

func (s *importExternalUsersSuite) newImportExternalUsersOperation() *importExternalUsersOperation {
	return &importExternalUsersOperation{
		service: s.service,
	}
}

func (s *importExternalUsersSuite) TestRegisterExternalUsersImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterExternalUsersImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
}

// TestImportExternalUsersNoUsers verifies that the operation is a no-op when
// the model has no users.
func (s *importExternalUsersSuite) TestImportExternalUsersNoUsers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	op := s.newImportExternalUsersOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportExternalUsersOnlyLocalUsers verifies that the operation is a no-op
// when all model users are local (no external users to create).
func (s *importExternalUsersSuite) TestImportExternalUsersOnlyLocalUsers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddUser(description.UserArgs{
		Name:        "admin",
		Access:      string(permission.AdminAccess),
		CreatedBy:   "admin",
		DateCreated: time.Now(),
	})

	op := s.newImportExternalUsersOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportExternalUsersCreatesExternalUsers verifies that external users from
// the model description are passed to ImportExternalUsers with their display
// name and creation date preserved.
func (s *importExternalUsersSuite) TestImportExternalUsersCreatesExternalUsers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	extDate := time.Now().Add(-time.Hour).Truncate(time.Minute).UTC()
	model.AddUser(description.UserArgs{
		Name:        "admin",
		Access:      string(permission.AdminAccess),
		CreatedBy:   "admin",
		DateCreated: time.Now(),
	})
	model.AddUser(description.UserArgs{
		Name:           "bob@external",
		Access:         string(permission.ReadAccess),
		CreatedBy:      "bob@external",
		DateCreated:    extDate,
		DisplayName:    "Bob External",
		LastConnection: time.Now(),
	})

	bobExtName, err := user.NewName("bob@external")
	c.Assert(err, tc.ErrorIsNil)

	s.service.EXPECT().ImportExternalUsers(gomock.Any(), []internal.ExternalUserImport{
		{
			Name:        bobExtName,
			DisplayName: "Bob External",
			DateCreated: extDate,
		},
	}).Return(nil)

	op := s.newImportExternalUsersOperation()
	err = op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

type importOfferAccessSuite struct {
	coordinator *MockCoordinator
	service     *MockImportOfferAccessService
}

func TestImportOfferAccessSuite(t *testing.T) {
	tc.Run(t, &importOfferAccessSuite{})
}

func (s *importOfferAccessSuite) TestExecute(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{})
	offerUUID := tc.Must(c, uuid.NewUUID)
	offerArgs := description.ApplicationOfferArgs{
		OfferUUID: offerUUID.String(),
		ACL:       map[string]string{"admin": permission.AdminAccess.String()},
	}
	app.AddOffer(offerArgs)
	offerUUID2 := tc.Must(c, uuid.NewUUID)
	offerArgs2 := description.ApplicationOfferArgs{
		OfferUUID: offerUUID2.String(),
		ACL:       map[string]string{"george": permission.ConsumeAccess.String()},
	}
	app.AddOffer(offerArgs2)
	input := []access.OfferImportAccess{
		{
			UUID:   offerUUID,
			Access: map[string]permission.Access{"admin": permission.AdminAccess},
		}, {
			UUID:   offerUUID2,
			Access: map[string]permission.Access{"george": permission.ConsumeAccess},
		},
	}
	s.service.EXPECT().ImportOfferAccess(gomock.Any(), input).Return(nil)

	// Act
	err := s.newImportOfferAccessOperation().Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importOfferAccessSuite) TestRollback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{})
	offerUUID := tc.Must(c, uuid.NewUUID)
	offerArgs := description.ApplicationOfferArgs{
		OfferUUID: offerUUID.String(),
		ACL:       map[string]string{"admin": permission.AdminAccess.String()},
	}
	app.AddOffer(offerArgs)
	offerUUID2 := tc.Must(c, uuid.NewUUID)
	offerArgs2 := description.ApplicationOfferArgs{
		OfferUUID: offerUUID2.String(),
		ACL:       map[string]string{"george": permission.ConsumeAccess.String()},
	}
	app.AddOffer(offerArgs2)
	s.service.EXPECT().DeletePermissionsByGrantOnUUID(
		gomock.Any(), []string{offerUUID.String(), offerUUID2.String()}).Return(nil)

	// Act
	err := s.newImportOfferAccessOperation().Rollback(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importOfferAccessSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportOfferAccessService(ctrl)

	c.Cleanup(func() {
		s.coordinator = nil
		s.service = nil
	})

	return ctrl
}

func (s *importOfferAccessSuite) newImportOfferAccessOperation() *offerAccessImportOperation {
	return &offerAccessImportOperation{
		service: s.service,
	}
}
