// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	coretesting "github.com/juju/juju/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) service() *SecretService {
	return NewSecretService(s.state, loggo.GetLogger("test"), NotImplementedBackendConfigGetter)
}

type successfulToken struct{}

func (t successfulToken) Check() error {
	return nil
}

func ptr[T any](v T) *T {
	return &v
}

func (s *serviceSuite) TestCreateUserSecretURIs(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coretesting.ModelTag.Id(), nil)

	got, err := s.service().CreateSecretURIs(context.Background(), 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 2)
	c.Assert(got[0].SourceUUID, gc.Equals, coretesting.ModelTag.Id())
	c.Assert(got[1].SourceUUID, gc.Equals, coretesting.ModelTag.Id())
}

func (s *serviceSuite) TestCreateUserSecret(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	p := domainsecret.UpsertSecretParams{
		Description: "a secret",
		Label:       "my secret",
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   true,
	}
	s.state = NewMockState(ctrl)
	s.state.EXPECT().CreateUserSecret(gomock.Any(), 1, uri, p).Return(nil)

	err := s.service().CreateSecret(context.Background(), uri, CreateSecretParams{
		UpdateSecretParams: UpdateSecretParams{
			LeaderToken: successfulToken{},
			Description: ptr("a secret"),
			Label:       ptr("my secret"),
			Data:        map[string]string{"foo": "bar"},
			AutoPrune:   ptr(true),
		},
		Version:    1,
		UserSecret: true,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetSecret(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	md := &coresecrets.SecretMetadata{
		URI:   uri,
		Label: "my secret",
	}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecret(gomock.Any(), uri).Return(md, nil)

	got, err := s.service().GetSecret(context.Background(), uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, md)
}

func (s *serviceSuite) TestGetSecretValue(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretValue(gomock.Any(), uri, 666).Return(coresecrets.SecretData{"foo": "bar"}, nil, nil)

	data, ref, err := s.service().GetSecretValue(context.Background(), uri, 666)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ref, gc.IsNil)
	c.Assert(data, jc.DeepEquals, coresecrets.NewSecretValue(map[string]string{"foo": "bar"}))
}

/*
// TODO(secrets) - tests copied from facade which need to be re-implemented here
func (s *serviceSuite) TestGetSecretContentConsumerFirstTime(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check().Return(nil)
	s.expectGetAppOwnedOrUnitOwnedSecretMetadataNotFound()

	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Label: "label"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *serviceSuite) TestGetSecretContentConsumerUpdateLabel(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	s.expectSecretAccessQuery(1)

	s.expectGetAppOwnedOrUnitOwnedSecretMetadataNotFound()
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, names.NewUnitTag("mariadb/0")).Return(
		&coresecrets.SecretConsumerMetadata{
			Label:           "old-label",
			CurrentRevision: 668,
			LatestRevision:  668,
		}, nil,
	)
	s.secretsConsumer.EXPECT().SaveSecretConsumer(gomock.Any(),
		uri, names.NewUnitTag("mariadb/0"), &coresecrets.SecretConsumerMetadata{
			Label:           "new-label",
			CurrentRevision: 668,
			LatestRevision:  668,
		}).Return(nil)

	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Label: "new-label"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *serviceSuite) TestGetSecretContentConsumerFirstTimeUsingLabelFailed(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectGetAppOwnedOrUnitOwnedSecretMetadataNotFound()
	s.secretsConsumer.EXPECT().GetURIByConsumerLabel(gomock.Any(), "label-1", names.NewUnitTag("mariadb/0")).Return(nil, errors.NotFoundf("secret"))

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{Label: "label-1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `consumer label "label-1" not found`)
}
func (s *SecretsManagerSuite) TestGetSecretContentForAppSecretSameLabel(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.expectSecretAccessQuery(1)

	s.secretService.EXPECT().ListCharmSecrets(gomock.Any(), secretservice.CharmSecretOwners{
		UnitName:        ptr("mariadb/0"),
		ApplicationName: ptr("mariadb"),
	}).Return([]*coresecrets.SecretMetadata{
		{
			URI:            uri,
			LatestRevision: 668,
			Label:          "foo",
			OwnerTag:       names.NewApplicationTag("mariadb").String(),
		},
	}, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 668,
		},
	}}, nil)

	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, s.authTag).
		Return(nil, errors.NotFoundf("secret consumer"))
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{LatestRevision: 668}, nil)
	s.secretsConsumer.EXPECT().SaveSecretConsumer(gomock.Any(),
		uri, names.NewUnitTag("mariadb/0"), &coresecrets.SecretConsumerMetadata{LatestRevision: 668, CurrentRevision: 668}).Return(nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Label: "foo"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestUpdateSecretDuplicateLabel(c *gc.C) {
	defer s.setup(c).Finish()

	p := secretservice.UpdateSecretParams{
		LeaderToken: s.token,
		Label:       ptr("foobar"),
	}
	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().UpdateSecret(gomock.Any(), &expectURI, p).Return(
		nil, fmt.Errorf("dup label %w", state.LabelExists),
	)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check().Return(nil)
	s.secretService.EXPECT().GetSecret(context.Background(), uri).Return(&coresecrets.SecretMetadata{}, nil)
	s.expectSecretAccessQuery(2)

	results, err := s.facade.UpdateSecrets(context.Background(), params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URI: uri.String(),
			UpsertSecretArg: params.UpsertSecretArg{
				Label: ptr("foobar"),
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: `secret with label "foobar" already exists`, Code: params.CodeAlreadyExists},
		}},
	})
}
func (s *SecretsManagerSuite) TestSecretsRotatedThenNever(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{
		OwnerTag:       "application-mariadb",
		RotatePolicy:   coresecrets.RotateNever,
		LatestRevision: 667,
	}, nil)

	result, err := s.facade.SecretsRotated(context.Background(), params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.ID,
			OriginalRevision: 666,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentForUnitOwnedSecretUpdateLabel(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	md := coresecrets.SecretMetadata{
		URI:            uri,
		LatestRevision: 668,
		Label:          "foz",
		OwnerTag:       s.authTag.String(),
	}

	s.expectSecretAccessQuery(1)

	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "foo", gomock.Any()).Return(uri, nil, nil)

	// Label is updated on owner metadata, not consumer metadata since it is a secret owned by the caller.
	s.secretService.EXPECT().UpdateSecret(gomock.Any(), uri, gomock.Any()).DoAndReturn(
		func(_ context.Context, uri *coresecrets.URI, p secretservice.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
			c.Assert(p.LeaderToken, gc.NotNil)
			c.Assert(p.LeaderToken.Check(), jc.ErrorIsNil)
			c.Assert(p.Label, gc.NotNil)
			c.Assert(*p.Label, gc.Equals, "foo")
			return nil, nil
		},
	)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, secretservice.SecretConsumer{
		UnitName: ptr("mariadb/0"),
	}, false, false, nil).
		Return(668, nil)

	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Label: "foo"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}
*/
