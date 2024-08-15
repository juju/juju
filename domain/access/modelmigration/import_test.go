// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/description/v8"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
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

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestNoModelUserPermissions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	modelUUID := model.Tag().Id()
	modelID := permission.ID{
		ObjectType: permission.Model,
		Key:        modelUUID,
	}
	steveTag := names.NewUserTag("steve")
	bobTag := names.NewUserTag("bob")
	bobName := user.NameFromTag(bobTag)
	bobTime := time.Now().Round(time.Minute).UTC()
	bob := description.UserArgs{
		Name:           bobTag,
		Access:         string(permission.AdminAccess),
		CreatedBy:      steveTag,
		DateCreated:    time.Now(),
		DisplayName:    bobTag.Name(),
		LastConnection: bobTime,
	}
	bazzaTag := names.NewUserTag("bazza")
	bazzaName := user.NameFromTag(bazzaTag)
	bazzaTime := bobTime.Add(time.Minute)
	bazza := description.UserArgs{
		Name:           bazzaTag,
		Access:         string(permission.ReadAccess),
		CreatedBy:      bobTag,
		DateCreated:    time.Now(),
		DisplayName:    bazzaTag.Name(),
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
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}
