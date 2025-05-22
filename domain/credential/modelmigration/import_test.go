// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"regexp"
	"testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	usertesting "github.com/juju/juju/core/user/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestEmptyCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	// No import executed.
	s.service.EXPECT().UpdateCloudCredential(gomock.All(), gomock.Any(), gomock.Any()).Times(0)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:      "fred",
			Cloud:      "cirrus",
			Name:       "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
	)
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"hello": "world"})
	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cloud.Credential{}, coreerrors.NotFound)
	s.service.EXPECT().UpdateCloudCredential(gomock.Any(), key, cred).Times(1)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportExistingMatches(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:      "fred",
			Cloud:      "cirrus",
			Name:       "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
	)
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"hello": "world"})
	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cred, nil)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportExistingAuthTypeMisMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:      "fred",
			Cloud:      "cirrus",
			Name:       "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
	)
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"hello": "world"})
	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cred, nil)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, `credential auth type mismatch: "access-key" != "userpass"`)
}

func (s *importSuite) TestImportExistingAttributesMisMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.SetCloudCredential(
		description.CloudCredentialArgs{
			Owner:      "fred",
			Cloud:      "cirrus",
			Name:       "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
	)
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"goodbye": "world"})
	key := credential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.service.EXPECT().CloudCredential(gomock.All(), key).Times(1).Return(cred, nil)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`credential attribute mismatch: map[goodbye:world] != map[hello:world]`))
}
