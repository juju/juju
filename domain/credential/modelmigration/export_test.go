// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v4"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
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

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"foo": "bar"}, false)
	s.service.EXPECT().CloudCredential(gomock.Any(), tag).
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

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	s.service.EXPECT().CloudCredential(gomock.Any(), tag).
		Times(1).
		Return(cloud.Credential{}, errors.NotFound)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.ErrorMatches, "not found")
}
