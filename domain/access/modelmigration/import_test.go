// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accesserrors "github.com/juju/juju/domain/access/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

func TestImportSuite(t *stdtesting.T) { tc.Run(t, &importSuite{}) }
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

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
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
