// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type UniterSecretsSuite struct {
	testhelpers.IsolationSuite

	authorizer *facademocks.MockAuthorizer

	leadership    *MockChecker
	secretService *MockSecretService
	authTag       names.Tag
	clock         clock.Clock

	facade *UniterAPI
}

func TestUniterSecretsSuite(t *testing.T) {
	tc.Run(t, &UniterSecretsSuite{})
}

func (s *UniterSecretsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUnitTag("mariadb/0")
}

func (s *UniterSecretsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)

	s.leadership = NewMockChecker(ctrl)
	s.secretService = NewMockSecretService(ctrl)
	s.expectAuthUnitAgent()

	s.clock = testclock.NewClock(time.Now())

	var err error
	s.facade, err = NewTestAPI(c, s.authorizer, s.leadership, s.secretService, nil, s.clock)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

func (s *UniterSecretsSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
	s.authorizer.EXPECT().GetAuthTag().Return(s.authTag).AnyTimes()
}

func (s *UniterSecretsSuite) TestCreateCharmSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	data := map[string]string{"foo": "bar"}
	checksum, err := coresecrets.NewSecretValue(data).Checksum()
	c.Assert(err, tc.ErrorIsNil)

	p := secretservice.CreateCharmSecretParams{
		Version:    secrets.Version,
		CharmOwner: secretservice.CharmSecretOwner{Kind: secretservice.ApplicationOwner, ID: "mariadb"},
		UpdateCharmSecretParams: secretservice.UpdateCharmSecretParams{
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   "mariadb/0",
			},
			RotatePolicy: ptr(coresecrets.RotateDaily),
			ExpireTime:   ptr(s.clock.Now()),
			Description:  ptr("my secret"),
			Label:        ptr("foobar"),
			Params:       map[string]interface{}{"param": 1},
			Data:         data,
			Checksum:     checksum,
		},
	}
	var gotURI *coresecrets.URI
	s.secretService.EXPECT().CreateCharmSecret(gomock.Any(), gomock.Any(), p).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, p secretservice.CreateCharmSecretParams) error {
			gotURI = uri
			return nil
		},
	)

	results, err := s.facade.createSecrets(c.Context(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(s.clock.Now()),
				Description:  ptr("my secret"),
				Label:        ptr("foobar"),
				Params:       map[string]interface{}{"param": 1},
				Content:      params.SecretContentParams{Data: data, Checksum: checksum},
			},
		}, {
			UpsertSecretArg: params.UpsertSecretArg{},
		}, {
			OwnerTag: "application-mysql",
			UpsertSecretArg: params.UpsertSecretArg{
				Content: params.SecretContentParams{Data: data},
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: gotURI.String(),
		}, {
			Error: &params.Error{Message: `empty secret value not valid`, Code: params.CodeNotValid},
		}, {
			Error: &params.Error{Message: `permission denied`, Code: params.CodeUnauthorized},
		}},
	})
}

func (s *UniterSecretsSuite) TestCreateCharmSecretDuplicateLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	p := secretservice.CreateCharmSecretParams{
		Version:    secrets.Version,
		CharmOwner: secretservice.CharmSecretOwner{Kind: secretservice.ApplicationOwner, ID: "mariadb"},
		UpdateCharmSecretParams: secretservice.UpdateCharmSecretParams{
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   "mariadb/0",
			},
			Label: ptr("foobar"),
			Data:  map[string]string{"foo": "bar"},
		},
	}
	s.secretService.EXPECT().CreateCharmSecret(gomock.Any(), gomock.Any(), p).Return(
		fmt.Errorf("dup label %w", secreterrors.SecretLabelAlreadyExists),
	)

	results, err := s.facade.createSecrets(c.Context(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				Label:   ptr("foobar"),
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Error: &params.Error{Message: `secret with label "foobar" already exists`, Code: params.CodeAlreadyExists},
		}},
	})
}

func (s *UniterSecretsSuite) TestUpdateSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	data := map[string]string{"foo": "bar"}
	checksum, err := coresecrets.NewSecretValue(data).Checksum()
	c.Assert(err, tc.ErrorIsNil)

	p := secretservice.UpdateCharmSecretParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		RotatePolicy: ptr(coresecrets.RotateDaily),
		ExpireTime:   ptr(s.clock.Now()),
		Description:  ptr("my secret"),
		Label:        ptr("foobar"),
		Params:       map[string]interface{}{"param": 1},
		Data:         data,
		Checksum:     checksum,
	}
	pWithBackendId := p
	p.ValueRef = &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}
	p.Data = nil
	p.Checksum = ""
	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().UpdateCharmSecret(gomock.Any(), &expectURI, p).Return(nil)
	s.secretService.EXPECT().UpdateCharmSecret(gomock.Any(), &expectURI, pWithBackendId).Return(nil)

	results, err := s.facade.updateSecrets(c.Context(), params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URI: uri.String(),
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(s.clock.Now()),
				Description:  ptr("my secret"),
				Label:        ptr("foobar"),
				Params:       map[string]interface{}{"param": 1},
				Content:      params.SecretContentParams{Data: data, Checksum: checksum},
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}, {}, {
			Error: &params.Error{Message: `at least one attribute to update must be specified`},
		}},
	})
}

func (s *UniterSecretsSuite) TestRemoveSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
	}).Return(nil)

	results, err := s.facade.removeSecrets(c.Context(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI: expectURI.String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *UniterSecretsSuite) TestRemoveSecretRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	}).Return(nil)

	results, err := s.facade.removeSecrets(c.Context(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *UniterSecretsSuite) TestRemoveSecretNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	}).Return(secreterrors.SecretNotFound)

	results, err := s.facade.removeSecrets(c.Context(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.Satisfies, params.IsCodeSecretNotFound)
}

func (s *UniterSecretsSuite) TestSecretsGrant(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), uri, secretservice.SecretAccessParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Scope:   secretservice.SecretAccessScope{Kind: secretservice.RelationAccessScope, ID: "wordpress:db mysql:server"},
		Subject: secretservice.SecretAccessor{Kind: secretservice.UnitAccessor, ID: "wordpress/0"},
		Role:    coresecrets.RoleView,
	}).Return(errors.New("boom"))

	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	result, err := s.facade.secretsGrant(c.Context(), params.GrantRevokeSecretArgs{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
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

func (s *UniterSecretsSuite) TestSecretsRevoke(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().RevokeSecretAccess(gomock.Any(), uri, secretservice.SecretAccessParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Scope:   secretservice.SecretAccessScope{Kind: secretservice.RelationAccessScope, ID: "wordpress:db mysql:server"},
		Subject: secretservice.SecretAccessor{Kind: secretservice.UnitAccessor, ID: "wordpress/0"},
		Role:    coresecrets.RoleView,
	}).Return(errors.New("boom"))

	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	result, err := s.facade.secretsRevoke(c.Context(), params.GrantRevokeSecretArgs{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
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

func (s *UniterSecretsSuite) TestUpdateTrackedRevisions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetConsumedRevision(gomock.Any(), uri, coreunit.GenName(c, "mariadb/0"), true, false, nil).
		Return(668, nil)
	result, err := s.facade.updateTrackedRevisions(c.Context(), []string{uri.ID})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}
