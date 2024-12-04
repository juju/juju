// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
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
	dst.SetCloudCredential(description.CloudCredentialArgs{
		Owner: names.NewUserTag("fred"),
		Cloud: names.NewCloudTag("cirrus"),
		Name:  "foo",
	})

	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"foo": "bar"}, false)
	s.service.EXPECT().CloudCredential(gomock.Any(), key).
		Times(1).
		Return(cred, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	got := dst.CloudCredential()
	c.Assert(got, gc.NotNil)
}

func (s *exportSuite) TestExportNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})
	dst.SetCloudCredential(description.CloudCredentialArgs{
		Owner: names.NewUserTag("fred"),
		Cloud: names.NewCloudTag("cirrus"),
		Name:  "foo",
	})

	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.Any(), key).
		Times(1).
		Return(cloud.Credential{}, coreerrors.NotFound)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.ErrorMatches, "not found")
}
