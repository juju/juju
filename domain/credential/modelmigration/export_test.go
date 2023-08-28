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

	// TODO(wallyworld) - tag not used yet.
	var tag names.CloudCredentialTag
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"foo": "bar"}, false)
	s.service.EXPECT().CloudCredential(gomock.Any(), tag).
		Times(1).
		Return(cred, nil)

	// Assert that the destination description model has no
	// credentials before the migration:
	c.Assert(dst.CloudCredential(), gc.IsNil)
	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	got := dst.CloudCredential()
	c.Assert(got, gc.NotNil)
}

func (s *exportSuite) TestExportNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	var tag names.CloudCredentialTag
	s.service.EXPECT().CloudCredential(gomock.Any(), tag).
		Times(1).
		Return(cloud.Credential{}, errors.NotFound)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.ErrorMatches, "not found")
}
