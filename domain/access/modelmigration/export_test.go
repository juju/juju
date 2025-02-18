// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
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

func (s *exportSuite) TestExport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	bobTag := names.NewUserTag("bob")
	bobName := user.NameFromTag(bobTag)
	bazzaTag := names.NewUserTag("bazza")
	bazzaName := user.NameFromTag(bazzaTag)
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
		Key:        dst.Tag().Id(),
	}).Return(userAccesses, nil)

	bobTime := time.Now().Truncate(time.Minute).UTC()
	bazzaTime := time.Now().Truncate(time.Minute).UTC().Add(-time.Minute)
	s.service.EXPECT().LastModelLogin(
		gomock.Any(), bobName, coremodel.UUID(dst.Tag().Id()),
	).Return(bobTime, nil)
	s.service.EXPECT().LastModelLogin(
		gomock.Any(), bazzaName, coremodel.UUID(dst.Tag().Id()),
	).Return(bazzaTime, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	users := dst.Users()
	c.Assert(users, gc.HasLen, 2)
	c.Check(users[0].Name(), gc.Equals, names.NewUserTag(userAccesses[0].UserName.Name()))
	c.Check(users[0].Access(), gc.Equals, string(userAccesses[0].Access))
	c.Check(users[0].CreatedBy(), gc.Equals, names.NewUserTag(userAccesses[0].CreatedBy.Name()))
	c.Check(users[0].DateCreated(), gc.Equals, userAccesses[0].DateCreated)
	c.Check(users[0].DisplayName(), gc.Equals, userAccesses[0].DisplayName)
	c.Check(users[0].LastConnection(), gc.Equals, bazzaTime)
	c.Check(users[1].Name(), gc.Equals, names.NewUserTag(userAccesses[1].UserName.Name()))
	c.Check(users[1].Access(), gc.Equals, string(userAccesses[1].Access))
	c.Check(users[1].CreatedBy(), gc.Equals, names.NewUserTag(userAccesses[1].CreatedBy.Name()))
	c.Check(users[1].DateCreated(), gc.Equals, userAccesses[1].DateCreated)
	c.Check(users[1].DisplayName(), gc.Equals, userAccesses[1].DisplayName)
	c.Check(users[1].LastConnection(), gc.Equals, bobTime)
}
