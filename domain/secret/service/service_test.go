// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
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
		Description: ptr("a secret"),
		Label:       ptr("my secret"),
		Data:        coresecrets.SecretData{"foo": "bar"},
		AutoPrune:   ptr(true),
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

func (s *serviceSuite) TestUpdateSecretNoRotate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	exipreTime := time.Now()
	uri := coresecrets.NewURI()
	p := domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateNever),
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         coresecrets.SecretData{"foo": "bar"},
		AutoPrune:    ptr(true),
		ExpireTime:   ptr(exipreTime),
	}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, p).Return(nil)

	err := s.service().UpdateSecret(context.Background(), uri, UpdateSecretParams{
		LeaderToken: successfulToken{},
		Description: ptr("a secret"),
		Label:       ptr("my secret"),
		Data:        map[string]string{"foo": "bar"},
		AutoPrune:   ptr(true),
		ExpireTime:  ptr(exipreTime),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateSecret(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	p := domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateDaily),
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         coresecrets.SecretData{"foo": "bar"},
		AutoPrune:    ptr(true),
	}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, p).Return(nil)

	err := s.service().UpdateSecret(context.Background(), uri, UpdateSecretParams{
		LeaderToken:  successfulToken{},
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         map[string]string{"foo": "bar"},
		AutoPrune:    ptr(true),
		RotatePolicy: ptr(coresecrets.RotateDaily),
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

func (s *serviceSuite) TestGetSecretConsumer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mysql/0").Return(consumer, 666, nil)

	got, err := s.service().GetSecretConsumer(context.Background(), uri, "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)
}

func (s *serviceSuite) TestGetSecretConsumerAndLatest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mysql/0").Return(consumer, 666, nil)

	got, latest, err := s.service().GetSecretConsumerAndLatest(context.Background(), uri, "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)
	c.Assert(latest, gc.Equals, 666)
}

func (s *serviceSuite) TestSaveSecretConsumer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mysql/0", consumer).Return(nil)

	err := s.service().SaveSecretConsumer(context.Background(), uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetUserSecretURIByLabel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()

	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetUserSecretURIByLabel(gomock.Any(), "my label").Return(uri, nil)

	got, err := s.service().GetUserSecretURIByLabel(context.Background(), "my label")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, uri)
}

func (s *serviceSuite) TestListUserSecrets(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	rev := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().ListUserSecrets(gomock.Any()).Return(md, rev, nil)

	gotSecrets, gotRevisions, err := s.service().ListUserSecrets(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSecrets, jc.DeepEquals, md)
	c.Assert(gotRevisions, jc.DeepEquals, rev)
}

func (s *serviceSuite) TestListCharmSecrets(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	owners := []CharmSecretOwner{{
		Kind: ApplicationOwner,
		ID:   "mysql",
	}, {
		Kind: UnitOwner,
		ID:   "mysql/0",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	rev := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"}).
		Return(md, rev, nil)

	gotSecrets, gotRevisions, err := s.service().ListCharmSecrets(context.Background(), owners...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSecrets, jc.DeepEquals, md)
	c.Assert(gotRevisions, jc.DeepEquals, rev)
}

func (s *serviceSuite) TestListCharmJustApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	owners := []CharmSecretOwner{{
		Kind: ApplicationOwner,
		ID:   "mysql",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	rev := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}, domainsecret.NilUnitOwners).
		Return(md, rev, nil)

	gotSecrets, gotRevisions, err := s.service().ListCharmSecrets(context.Background(), owners...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSecrets, jc.DeepEquals, md)
	c.Assert(gotRevisions, jc.DeepEquals, rev)
}

func (s *serviceSuite) TestListCharmJustUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	owners := []CharmSecretOwner{{
		Kind: UnitOwner,
		ID:   "mysql/0",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	rev := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state = NewMockState(ctrl)
	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.NilApplicationOwners, domainsecret.UnitOwners{"mysql/0"}).
		Return(md, rev, nil)

	gotSecrets, gotRevisions, err := s.service().ListCharmSecrets(context.Background(), owners...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSecrets, jc.DeepEquals, md)
	c.Assert(gotRevisions, jc.DeepEquals, rev)
}

func (s *serviceSuite) TestGetURIByConsumerLabel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetURIByConsumerLabel(gomock.Any(), "my label", "mysql/0").Return(uri, nil)

	got, err := s.service().GetURIByConsumerLabel(context.Background(), "my label", "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, uri)
}

func (s *serviceSuite) TestUpdateRemoteSecretRevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri, 666).Return(nil)

	err := s.service().UpdateRemoteSecretRevision(context.Background(), uri, 666)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(&coresecrets.SecretConsumerMetadata{}, 666, nil)

	got, err := s.service().UpdateRemoteConsumedRevision(context.Background(), uri, "remote-app/0", false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevisionRefresh(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(&coresecrets.SecretConsumerMetadata{}, 666, nil)
	s.state.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0", consumer).Return(nil)

	got, err := s.service().UpdateRemoteConsumedRevision(context.Background(), uri, "remote-app/0", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevisionFirstTimeRefresh(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.state.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0", consumer).Return(nil)

	got, err := s.service().UpdateRemoteConsumedRevision(context.Background(), uri, "remote-app/0", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *serviceSuite) TestGrantSecretUnitAccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeID:       "mysql",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleManage,
	}).Return(nil)

	err := s.service().GrantSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Scope: SecretAccessScope{
			Kind: ApplicationAccessScope,
			ID:   "mysql",
		},
		Subject: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mysql/0",
		},
		Role: "manage",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretApplicationAccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeID:       "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}).Return(nil)

	err := s.service().GrantSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Scope: SecretAccessScope{
			Kind: ApplicationAccessScope,
			ID:   "mysql",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "mysql",
		},
		Role: "view",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretModelAccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeModel,
		SubjectTypeID: domainsecret.SubjectModel,
		RoleID:        domainsecret.RoleManage,
	}).Return(nil)

	err := s.service().GrantSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Scope: SecretAccessScope{
			Kind: ModelAccessScope,
		},
		Subject: SecretAccessor{
			Kind: ModelAccessor,
		},
		Role: "manage",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretRelationScope(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}).Return(nil)

	err := s.service().GrantSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Scope: SecretAccessScope{
			Kind: RelationAccessScope,
			ID:   "mysql:db mediawiki:db",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "mysql",
		},
		Role: "view",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretUnitAccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}).Return(nil)

	err := s.service().RevokeSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Subject: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mysql/0",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretApplicationAccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(nil)

	err := s.service().RevokeSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "mysql",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretModelAccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
	}).Return(nil)

	err := s.service().RevokeSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Subject: SecretAccessor{
			Kind: ModelAccessor,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetSecretAccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return("manage", nil)

	role, err := s.service().GetSecretAccess(context.Background(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, coresecrets.RoleManage)
}

func (s *serviceSuite) TestGetSecretAccessNone(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return("", nil)

	role, err := s.service().GetSecretAccess(context.Background(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, coresecrets.RoleNone)
}

func (s *serviceSuite) TestGetSecretAccessApplicationScope(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccessScope(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(&domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeApplication,
		ScopeID:     "mysql",
	}, nil)

	scope, err := s.service().GetSecretAccessScope(context.Background(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scope, jc.DeepEquals, SecretAccessScope{
		Kind: ApplicationAccessScope,
		ID:   "mysql",
	})
}

func (s *serviceSuite) TestGetSecretAccessRelationScope(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretAccessScope(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(&domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeRelation,
		ScopeID:     "mysql:db mediawiki:db",
	}, nil)

	scope, err := s.service().GetSecretAccessScope(context.Background(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scope, jc.DeepEquals, SecretAccessScope{
		Kind: RelationAccessScope,
		ID:   "mysql:db mediawiki:db",
	})
}

func (s *serviceSuite) TestGetSecretGrants(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state = NewMockState(ctrl)
	s.state.EXPECT().GetSecretGrants(gomock.Any(), uri, coresecrets.RoleView).Return([]domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       "model-uuid",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}}, nil)

	g, err := s.service().GetSecretGrants(context.Background(), uri, coresecrets.RoleView)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(g, jc.DeepEquals, []SecretAccess{{
		Scope: SecretAccessScope{
			Kind: ModelAccessScope,
			ID:   "model-uuid",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "mysql",
		},
		Role: coresecrets.RoleView,
	}})
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

func (s *serviceSuite) TestWatchObsolete(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.state = NewMockState(ctrl)
	mockWatcherFactory := NewMockWatcherFactory(ctrl)

	ch := make(chan []string)
	mockStringWatcher := NewMockStringsWatcher(ctrl)
	mockStringWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	mockStringWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockStringWatcher.EXPECT().Kill().AnyTimes()

	var namespaceQuery eventsource.NamespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return []string{"revision-uuid-1", "revision-uuid-2"}, nil
	}
	s.state.EXPECT().InitialWatchStatementForObsoleteRevision(gomock.Any(),
		domainsecret.ApplicationOwners([]string{"mysql"}),
		domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"}),
	).Return("table", namespaceQuery)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher("table", changestream.Update, gomock.Any()).Return(mockStringWatcher, nil)

	gomock.InOrder(
		s.state.EXPECT().GetRevisionIDsForObsolete(gomock.Any(),
			domainsecret.ApplicationOwners([]string{"mysql"}),
			domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"}),
			"revision-uuid-1", "revision-uuid-2",
		).Return([]string{"yyy/1", "yyy/2"}, nil),
	)

	svc := NewWatchableService(s.state, coretesting.NewCheckLogger(c), mockWatcherFactory, nil)
	w, err := svc.WatchObsolete(context.Background(),
		CharmSecretOwner{
			Kind: ApplicationOwner,
			ID:   "mysql",
		},
		CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mysql/0",
		},
		CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mysql/1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)
	wC := watchertest.NewStringsWatcherC(c, w)

	select {
	case ch <- []string{"revision-uuid-1", "revision-uuid-2"}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
	}

	wC.AssertChange(
		"yyy/1",
		"yyy/2",
	)
	wC.AssertNoChange()
}
