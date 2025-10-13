// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coresecrets "github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
)

type secretsServiceSuite struct {
	baseSuite
}

func TestSecretsServiceSuite(t *testing.T) {
	tc.Run(t, &secretsServiceSuite{})
}

func (s *secretsServiceSuite) TestUpdateRemoteSecretRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.modelState.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri, 666).Return(nil)

	service := s.service(c)

	err := service.UpdateRemoteSecretRevision(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretsServiceSuite) TestUpdateRemoteConsumedRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.modelState.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(&coresecrets.SecretConsumerMetadata{}, 666, nil)

	service := s.service(c)

	got, err := service.UpdateRemoteConsumedRevision(c.Context(), uri, "remote-app/0", false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, 666)
}

func (s *secretsServiceSuite) TestUpdateRemoteConsumedRevisionRefresh(c *tc.C) {
	defer s.setupMocks(c).Finish()

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	uri := coresecrets.NewURI()
	s.modelState.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(&coresecrets.SecretConsumerMetadata{}, 666, nil)
	s.modelState.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0", consumer)

	service := s.service(c)

	got, err := service.UpdateRemoteConsumedRevision(c.Context(), uri, "remote-app/0", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, 666)
}

func (s *secretsServiceSuite) TestUpdateRemoteConsumedRevisionFirstTimeRefresh(c *tc.C) {
	defer s.setupMocks(c).Finish()

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	uri := coresecrets.NewURI()
	s.modelState.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.modelState.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0", consumer)

	service := s.service(c)

	got, err := service.UpdateRemoteConsumedRevision(c.Context(), uri, "remote-app/0", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, 666)
}
