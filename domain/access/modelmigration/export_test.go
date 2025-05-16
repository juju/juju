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
	usertesting "github.com/juju/juju/core/user/testing"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

func TestExportSuite(t *stdtesting.T) { tc.Run(t, &exportSuite{}) }
func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() *exportOperation {
	return &exportOperation{
		service: s.service,
	}
}

func (s *exportSuite) TestExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	bobName := usertesting.GenNewName(c, "bob")
	bazzaName := usertesting.GenNewName(c, "bazza")
	steveName := usertesting.GenNewName(c, "steve")

	userAccesses := []permission.UserAccess{{
		Access:      permission.ReadAccess,
		CreatedBy:   bobName,
		DateCreated: time.Now(),
		DisplayName: bazzaName.Name(),
		UserName:    bazzaName,
	}, {
		Access:      permission.AdminAccess,
		CreatedBy:   steveName,
		DateCreated: time.Now(),
		DisplayName: bobName.Name(),
		UserName:    bobName,
	}}

	s.service.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), permission.ID{
		ObjectType: permission.Model,
		Key:        dst.UUID(),
	}).Return(userAccesses, nil)

	bobTime := time.Now().Truncate(time.Minute).UTC()
	bazzaTime := time.Now().Truncate(time.Minute).UTC().Add(-time.Minute)
	s.service.EXPECT().LastModelLogin(
		gomock.Any(), bobName, coremodel.UUID(dst.UUID()),
	).Return(bobTime, nil)
	s.service.EXPECT().LastModelLogin(
		gomock.Any(), bazzaName, coremodel.UUID(dst.UUID()),
	).Return(bazzaTime, nil)

	op := s.newExportOperation()
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	users := dst.Users()
	c.Assert(users, tc.HasLen, 2)
	c.Check(users[0].Name(), tc.Equals, userAccesses[0].UserName.Name())
	c.Check(users[0].Access(), tc.Equals, string(userAccesses[0].Access))
	c.Check(users[0].CreatedBy(), tc.Equals, userAccesses[0].CreatedBy.Name())
	c.Check(users[0].DateCreated(), tc.Equals, userAccesses[0].DateCreated)
	c.Check(users[0].DisplayName(), tc.Equals, userAccesses[0].DisplayName)
	c.Check(users[0].LastConnection(), tc.Equals, bazzaTime)
	c.Check(users[1].Name(), tc.Equals, userAccesses[1].UserName.Name())
	c.Check(users[1].Access(), tc.Equals, string(userAccesses[1].Access))
	c.Check(users[1].CreatedBy(), tc.Equals, userAccesses[1].CreatedBy.Name())
	c.Check(users[1].DateCreated(), tc.Equals, userAccesses[1].DateCreated)
	c.Check(users[1].DisplayName(), tc.Equals, userAccesses[1].DisplayName)
	c.Check(users[1].LastConnection(), tc.Equals, bobTime)
}
