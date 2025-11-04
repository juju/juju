// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/secret"
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
	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.modelState.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri, 666, appUUID.String()).Return(nil)

	service := s.service(c)

	err := service.UpdateRemoteSecretRevision(c.Context(), uri, 666, appUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretsServiceSuite) TestSaveRemoteSecretConsumer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	appUUID := tc.Must(c, coreapplication.NewUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	relUUID := tc.Must(c, corerelation.NewUUID)
	md := coresecrets.SecretConsumerMetadata{}

	s.modelState.EXPECT().GetUnitUUID(gomock.Any(), "foo/0").Return(unitUUID.String(), nil)
	s.modelState.EXPECT().SaveRemoteSecretConsumer(
		gomock.Any(), uri, unitUUID.String(), md, appUUID.String(), relUUID.String()).Return(nil)

	service := s.service(c)

	unitName := unittesting.GenNewName(c, "foo/0")
	err := service.SaveRemoteSecretConsumer(c.Context(), uri, unitName, md, appUUID, relUUID)
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

	consumer := coresecrets.SecretConsumerMetadata{
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

	consumer := coresecrets.SecretConsumerMetadata{
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

func (s *secretsServiceSuite) TestProcessRemoteConsumerGetSecretNoPeekOrRefresh(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	consumer := unittesting.GenNewName(c, "consumer/0")
	data := map[string]string{"foo": "bar"}

	s.modelState.EXPECT().GetSecretAccess(gomock.Any(), uri, secret.AccessParams{
		SubjectTypeID: secret.SubjectApplication,
		SubjectID:     consumer.Application(),
	}).Return(secret.RoleView.String(), nil)
	s.modelState.EXPECT().GetSecretValue(gomock.Any(), uri, 666).Return(data, nil, nil)

	service := s.service(c)

	content, valueRef, latest, err := service.ProcessRemoteConsumerGetSecret(
		c.Context(), uri, consumer, ptr(666), false, false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(content, tc.DeepEquals, coresecrets.NewSecretValue(data))
	c.Assert(valueRef, tc.IsNil)
	c.Assert(latest, tc.Equals, 0)
}

func (s *secretsServiceSuite) TestProcessRemoteConsumerGetSecretPeek(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	consumer := unittesting.GenNewName(c, "consumer/0")
	data := map[string]string{"foo": "bar"}

	s.modelState.EXPECT().GetSecretAccess(gomock.Any(), uri, secret.AccessParams{
		SubjectTypeID: secret.SubjectApplication,
		SubjectID:     consumer.Application(),
	}).Return(secret.RoleView.String(), nil)
	s.modelState.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, consumer.String()).
		Return(&coresecrets.SecretConsumerMetadata{
			CurrentRevision: 665,
		}, 666, nil)
	s.modelState.EXPECT().GetSecretValue(gomock.Any(), uri, 666).Return(data, nil, nil)

	service := s.service(c)

	content, valueRef, latest, err := service.ProcessRemoteConsumerGetSecret(
		c.Context(), uri, consumer, nil, true, false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(content, tc.DeepEquals, coresecrets.NewSecretValue(data))
	c.Assert(valueRef, tc.IsNil)
	c.Assert(latest, tc.Equals, 666)
}

func (s *secretsServiceSuite) TestProcessRemoteConsumerGetSecretRefresh(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	consumer := unittesting.GenNewName(c, "consumer/0")
	data := map[string]string{"foo": "bar"}

	s.modelState.EXPECT().GetSecretAccess(gomock.Any(), uri, secret.AccessParams{
		SubjectTypeID: secret.SubjectApplication,
		SubjectID:     consumer.Application(),
	}).Return(secret.RoleView.String(), nil)
	s.modelState.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, consumer.String()).
		Return(&coresecrets.SecretConsumerMetadata{
			CurrentRevision: 665,
			Label:           "foo",
		}, 666, nil)
	s.modelState.EXPECT().GetSecretValue(gomock.Any(), uri, 666).Return(data, nil, nil)
	s.modelState.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, consumer.String(), coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
		Label:           "foo",
	})

	service := s.service(c)

	content, valueRef, latest, err := service.ProcessRemoteConsumerGetSecret(
		c.Context(), uri, consumer, nil, false, true)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(content, tc.DeepEquals, coresecrets.NewSecretValue(data))
	c.Assert(valueRef, tc.IsNil)
	c.Assert(latest, tc.Equals, 666)
}

func (s *secretsServiceSuite) TestProcessRemoteConsumerGetSecretNoConsumerExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	consumer := unittesting.GenNewName(c, "consumer/0")
	ref := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}

	s.modelState.EXPECT().GetSecretAccess(gomock.Any(), uri, secret.AccessParams{
		SubjectTypeID: secret.SubjectApplication,
		SubjectID:     consumer.Application(),
	}).Return(secret.RoleView.String(), nil)
	s.modelState.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, consumer.String()).
		Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.modelState.EXPECT().GetSecretValue(gomock.Any(), uri, 666).Return(nil, ref, nil)
	s.modelState.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, consumer.String(), coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	})

	service := s.service(c)

	content, valueRef, latest, err := service.ProcessRemoteConsumerGetSecret(
		c.Context(), uri, consumer, nil, false, false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(content.EncodedValues(), tc.HasLen, 0)
	c.Assert(valueRef, tc.DeepEquals, ref)
	c.Assert(latest, tc.Equals, 666)
}

func (s *secretsServiceSuite) TestProcessRemoteConsumerGetSecretPermissionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	consumer := unittesting.GenNewName(c, "consumer/0")

	s.modelState.EXPECT().GetSecretAccess(gomock.Any(), uri, secret.AccessParams{
		SubjectTypeID: secret.SubjectApplication,
		SubjectID:     consumer.Application(),
	}).Return(secret.RoleNone.String(), nil)

	service := s.service(c)

	_, _, _, err := service.ProcessRemoteConsumerGetSecret(
		c.Context(), uri, consumer, nil, false, false)
	c.Assert(err, tc.ErrorIs, secreterrors.PermissionDenied)
}
