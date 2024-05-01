// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type UniterSecretsSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer

	token         *MockToken
	leadership    *MockChecker
	secretService *MockSecretService
	authTag       names.Tag
	clock         clock.Clock

	facade *UniterAPI
}

func ptr[T any](v T) *T {
	return &v
}

var _ = gc.Suite(&UniterSecretsSuite{})

func (s *UniterSecretsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUnitTag("mariadb/0")
}

func (s *UniterSecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)

	s.leadership = NewMockChecker(ctrl)
	s.token = NewMockToken(ctrl)
	s.secretService = NewMockSecretService(ctrl)
	s.expectAuthUnitAgent()

	s.clock = testclock.NewClock(time.Now())

	var err error
	s.facade, err = NewTestAPI(s.authorizer, s.leadership, s.secretService, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *UniterSecretsSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
	s.authorizer.EXPECT().GetAuthTag().Return(s.authTag).AnyTimes()
}

func (s *UniterSecretsSuite) TestCreateCharmSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	p := secretservice.CreateCharmSecretParams{
		Version:    secrets.Version,
		CharmOwner: secretservice.CharmSecretOwner{Kind: secretservice.ApplicationOwner, ID: "mariadb"},
		UpdateCharmSecretParams: secretservice.UpdateCharmSecretParams{
			LeaderToken:  s.token,
			RotatePolicy: ptr(coresecrets.RotateDaily),
			ExpireTime:   ptr(s.clock.Now()),
			Description:  ptr("my secret"),
			Label:        ptr("foobar"),
			Params:       map[string]interface{}{"param": 1},
			Data:         map[string]string{"foo": "bar"},
		},
	}
	var gotURI *coresecrets.URI
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().CreateCharmSecret(gomock.Any(), gomock.Any(), p).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, p secretservice.CreateCharmSecretParams) error {
			gotURI = uri
			return nil
		},
	)

	results, err := s.facade.createSecrets(context.Background(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(s.clock.Now()),
				Description:  ptr("my secret"),
				Label:        ptr("foobar"),
				Params:       map[string]interface{}{"param": 1},
				Content:      params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}, {
			UpsertSecretArg: params.UpsertSecretArg{},
		}, {
			OwnerTag: "application-mysql",
			UpsertSecretArg: params.UpsertSecretArg{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: gotURI.String(),
		}, {
			Error: &params.Error{Message: `empty secret value not valid`, Code: params.CodeNotValid},
		}, {
			Error: &params.Error{Message: `permission denied`, Code: params.CodeUnauthorized},
		}},
	})
}

func (s *UniterSecretsSuite) TestCreateCharmSecretDuplicateLabel(c *gc.C) {
	defer s.setup(c).Finish()

	p := secretservice.CreateCharmSecretParams{
		Version:    secrets.Version,
		CharmOwner: secretservice.CharmSecretOwner{Kind: secretservice.ApplicationOwner, ID: "mariadb"},
		UpdateCharmSecretParams: secretservice.UpdateCharmSecretParams{
			LeaderToken: s.token,
			Label:       ptr("foobar"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().CreateCharmSecret(gomock.Any(), gomock.Any(), p).Return(
		fmt.Errorf("dup label %w", state.LabelExists),
	)

	results, err := s.facade.createSecrets(context.Background(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				Label:   ptr("foobar"),
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Error: &params.Error{Message: `secret with label "foobar" already exists`, Code: params.CodeAlreadyExists},
		}},
	})
}

func (s *UniterSecretsSuite) TestUpdateSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	p := secretservice.UpdateCharmSecretParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		RotatePolicy: ptr(coresecrets.RotateDaily),
		ExpireTime:   ptr(s.clock.Now()),
		Description:  ptr("my secret"),
		Label:        ptr("foobar"),
		Params:       map[string]interface{}{"param": 1},
		Data:         map[string]string{"foo": "bar"},
	}
	pWithBackendId := p
	p.ValueRef = &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}
	p.Data = nil
	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().UpdateCharmSecret(gomock.Any(), &expectURI, p).Return(nil)
	s.secretService.EXPECT().UpdateCharmSecret(gomock.Any(), &expectURI, pWithBackendId).Return(nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token).Times(2)

	results, err := s.facade.updateSecrets(context.Background(), params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URI: uri.String(),
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(s.clock.Now()),
				Description:  ptr("my secret"),
				Label:        ptr("foobar"),
				Params:       map[string]interface{}{"param": 1},
				Content:      params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}, {
			URI: uri.String(),
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(s.clock.Now()),
				Description:  ptr("my secret"),
				Label:        ptr("foobar"),
				Params:       map[string]interface{}{"param": 1},
				Content: params.SecretContentParams{ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				}},
			},
		}, {
			URI: uri.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}, {}, {
			Error: &params.Error{Message: `at least one attribute to update must be specified`},
		}},
	})
}

func (s *UniterSecretsSuite) TestRemoveSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
	}).Return(nil)

	results, err := s.facade.removeSecrets(context.Background(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI: expectURI.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *UniterSecretsSuite) TestRemoveSecretRevision(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	}).Return(nil)

	results, err := s.facade.removeSecrets(context.Background(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *UniterSecretsSuite) TestRemoveSecretNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	}).Return(secreterrors.SecretNotFound)

	results, err := s.facade.removeSecrets(context.Background(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, jc.Satisfies, params.IsCodeSecretNotFound)
}

func (s *UniterSecretsSuite) TestSecretsGrant(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), uri, secretservice.SecretAccessParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Scope:   secretservice.SecretAccessScope{Kind: secretservice.RelationAccessScope, ID: "wordpress:db mysql:server"},
		Subject: secretservice.SecretAccessor{Kind: secretservice.UnitAccessor, ID: "wordpress/0"},
		Role:    coresecrets.RoleView,
	}).Return(errors.New("boom"))
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	result, err := s.facade.secretsGrant(context.Background(), params.GrantRevokeSecretArgs{
		Args: []params.GrantRevokeSecretArg{{
			URI:         uri.String(),
			ScopeTag:    scopeTag.String(),
			SubjectTags: []string{subjectTag.String()},
			Role:        "view",
		}, {
			URI:      uri.String(),
			ScopeTag: scopeTag.String(),
			Role:     "bad",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: fmt.Sprintf(`cannot change access to %q for "unit-wordpress-0": boom`, uri.String())},
			},
			{
				Error: &params.Error{Code: params.CodeNotValid, Message: `secret role "bad" not valid`},
			},
		},
	})
}

func (s *UniterSecretsSuite) TestSecretsRevoke(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().RevokeSecretAccess(gomock.Any(), uri, secretservice.SecretAccessParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Scope:   secretservice.SecretAccessScope{Kind: secretservice.RelationAccessScope, ID: "wordpress:db mysql:server"},
		Subject: secretservice.SecretAccessor{Kind: secretservice.UnitAccessor, ID: "wordpress/0"},
		Role:    coresecrets.RoleView,
	}).Return(errors.New("boom"))
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	result, err := s.facade.secretsRevoke(context.Background(), params.GrantRevokeSecretArgs{
		Args: []params.GrantRevokeSecretArg{{
			URI:         uri.String(),
			ScopeTag:    scopeTag.String(),
			SubjectTags: []string{subjectTag.String()},
			Role:        "view",
		}, {
			URI:      uri.String(),
			ScopeTag: scopeTag.String(),
			Role:     "bad",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: fmt.Sprintf(`cannot change access to %q for "unit-wordpress-0": boom`, uri.String())},
			},
			{
				Error: &params.Error{Code: params.CodeNotValid, Message: `secret role "bad" not valid`},
			},
		},
	})
}

func (s *UniterSecretsSuite) TestUpdateTrackedRevisions(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", true, false, nil).
		Return(668, nil)
	result, err := s.facade.updateTrackedRevisions(context.Background(), []string{uri.ID})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}
