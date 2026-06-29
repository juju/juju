// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/credential"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	baseSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) service(c *tc.C) *Service {
	return NewService(s.state, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestImportModelCredentialCreatesMissing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "stratus", Owner: usertesting.GenNewName(c, "fred"), Name: "primary"}
	s.state.EXPECT().CloudCredential(gomock.Any(), key).
		Return(credential.CloudCredentialResult{}, credentialerrors.NotFound)
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), key, credential.CloudCredentialInfo{
		AuthType:   string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{"access-key": "key", "secret-key": "secret"},
		Revoked:    true,
	})

	got, err := s.service(c).ImportModelCredential(c.Context(), coremodelmigration.ModelCloudCredential{
		Cloud:      "stratus",
		Owner:      "fred",
		Name:       "primary",
		AuthType:   string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{"access-key": "key", "secret-key": "secret"},
		Revoked:    true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, key)
}

func (s *importSuite) TestImportModelCredentialExistingMatches(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "stratus", Owner: usertesting.GenNewName(c, "fred"), Name: "primary"}
	s.state.EXPECT().CloudCredential(gomock.Any(), key).Return(credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"username": "fred", "password": "secret"},
		},
	}, nil)

	_, err := s.service(c).ImportModelCredential(c.Context(), coremodelmigration.ModelCloudCredential{
		Cloud:      "stratus",
		Owner:      "fred",
		Name:       "primary",
		AuthType:   string(cloud.UserPassAuthType),
		Attributes: map[string]string{"username": "fred", "password": "secret"},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportModelCredentialExistingAuthTypeMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "stratus", Owner: usertesting.GenNewName(c, "fred"), Name: "primary"}
	s.state.EXPECT().CloudCredential(gomock.Any(), key).Return(credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType: string(cloud.UserPassAuthType),
		},
	}, nil)

	_, err := s.service(c).ImportModelCredential(c.Context(), coremodelmigration.ModelCloudCredential{
		Cloud:    "stratus",
		Owner:    "fred",
		Name:     "primary",
		AuthType: string(cloud.AccessKeyAuthType),
	})
	c.Assert(err, tc.ErrorMatches, `credential "stratus/fred/primary" auth type mismatch: "userpass" != "access-key"`)
}

func (s *importSuite) TestImportModelCredentialLookupError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := errors.New("boom")
	key := corecredential.Key{Cloud: "stratus", Owner: usertesting.GenNewName(c, "fred"), Name: "primary"}
	s.state.EXPECT().CloudCredential(gomock.Any(), key).Return(credential.CloudCredentialResult{}, expected)

	_, err := s.service(c).ImportModelCredential(c.Context(), coremodelmigration.ModelCloudCredential{
		Cloud: "stratus",
		Owner: "fred",
		Name:  "primary",
	})
	c.Assert(err, tc.ErrorIs, expected)
}
