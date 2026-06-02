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
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type UniterSecretsSuite struct {
	testhelpers.IsolationSuite

	authorizer *facademocks.MockAuthorizer

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

	s.secretService = NewMockSecretService(ctrl)
	s.expectAuthUnitAgent()

	s.clock = testclock.NewClock(time.Now())

	var err error
	s.facade, err = NewTestAPI(c, s.authorizer, s.secretService, nil, s.clock)
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

	p := secret.CreateCharmSecretParams{
		Version:    secrets.Version,
		CharmOwner: secret.CharmSecretOwner{Kind: secret.ApplicationCharmSecretOwner, ID: "mariadb"},
		UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
			Accessor: secret.SecretAccessor{
				Kind: secret.UnitAccessor,
				ID:   "mariadb/0",
			},
			RotatePolicy: new(coresecrets.RotateDaily),
			ExpireTime:   new(s.clock.Now()),
			Description:  new("my secret"),
			Label:        new("foobar"),
			Params:       map[string]any{"param": 1},
			Data:         data,
			Checksum:     checksum,
		},
	}
	var gotURI *coresecrets.URI
	s.secretService.EXPECT().CreateCharmSecret(gomock.Any(), gomock.Any(), p).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, p secret.CreateCharmSecretParams) error {
			gotURI = uri
			return nil
		},
	)

	results, err := s.facade.createSecrets(c.Context(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: new(coresecrets.RotateDaily),
				ExpireTime:   new(s.clock.Now()),
				Description:  new("my secret"),
				Label:        new("foobar"),
				Params:       map[string]any{"param": 1},
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

	p := secret.CreateCharmSecretParams{
		Version:    secrets.Version,
		CharmOwner: secret.CharmSecretOwner{Kind: secret.ApplicationCharmSecretOwner, ID: "mariadb"},
		UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
			Accessor: secret.SecretAccessor{
				Kind: secret.UnitAccessor,
				ID:   "mariadb/0",
			},
			Label: new("foobar"),
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
				Label:   new("foobar"),
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

	p := secret.UpdateCharmSecretParams{
		Accessor: secret.SecretAccessor{
			Kind: secret.UnitAccessor,
			ID:   "mariadb/0",
		},
		RotatePolicy: new(coresecrets.RotateDaily),
		ExpireTime:   new(s.clock.Now()),
		Description:  new("my secret"),
		Label:        new("foobar"),
		Params:       map[string]any{"param": 1},
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
				RotatePolicy: new(coresecrets.RotateDaily),
				ExpireTime:   new(s.clock.Now()),
				Description:  new("my secret"),
				Label:        new("foobar"),
				Params:       map[string]any{"param": 1},
				Content:      params.SecretContentParams{Data: data, Checksum: checksum},
			},
		}, {
			URI: uri.String(),
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: new(coresecrets.RotateDaily),
				ExpireTime:   new(s.clock.Now()),
				Description:  new("my secret"),
				Label:        new("foobar"),
				Params:       map[string]any{"param": 1},
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

func (s *UniterSecretsSuite) TestSecretsGrant(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), uri, secret.SecretAccessParams{
		Accessor: secret.SecretAccessor{
			Kind: secret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Scope:   secret.SecretAccessScope{Kind: secret.RelationAccessScope, ID: "wordpress:db mysql:server"},
		Subject: secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "wordpress/0"},
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

func (s *UniterSecretsSuite) TestUpdateTrackedRevisions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), true, false, nil).
		Return(668, nil)
	result, err := s.facade.updateTrackedRevisions(c.Context(), []string{uri.ID})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

// --- prepareSecretRevokes tests ---

func (s *UniterSecretsSuite) TestPrepareSecretRevokesInvalidURI(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	_, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         "not-a-valid-uri-%%%",
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorMatches, `.*invalid URL escape.*`)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesNotFoundSkipped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.SecretNotFound)

	result, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).
		Return(secreterrors.PermissionDenied)

	_, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorMatches, `revoking secrets access: permission denied`)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesMultipleSubjectsPartialFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)
	// Both subjects resolved in one call: first succeeds, second fails.
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{
			{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-1", SubjectTypeID: secret.SubjectApplication}},
			{Error: errors.New("resolve boom")},
		})

	_, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String(), names.NewApplicationTag("app2").String()},
	}})
	c.Assert(err, tc.ErrorMatches, `revoking secrets access: resolve boom`)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesOwnerFiltered(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri1, unitName).Return(nil)
	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri2, unitName).Return(nil)
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-1", SubjectTypeID: secret.SubjectApplication}}}).Times(2)
	// Only uri1 is returned by GetSecretOwnerKinds — uri2 was concurrently deleted.
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), gomock.Any()).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri1.ID,
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{
		{URI: uri1.String(), ScopeTag: names.NewRelationTag("one:db two:use").String(), SubjectTags: []string{names.NewApplicationTag("app1").String()}},
		{URI: uri2.String(), ScopeTag: names.NewRelationTag("one:db two:use").String(), SubjectTags: []string{names.NewApplicationTag("app2").String()}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.ID, tc.Equals, uri1.ID)
	c.Check(result[0].OwnerKind, tc.Equals, secret.ApplicationCharmSecretOwner)
}

func (s *UniterSecretsSuite) TestPrepareSecretRevokesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "mariadb/0")
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().CheckSecretManageAccess(gomock.Any(), uri, unitName).Return(nil)
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-app", SubjectTypeID: secret.SubjectApplication}}})
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri.ID,
			OwnerKind: secret.UnitCharmSecretOwner,
		}}, nil)

	result, err := s.facade.prepareSecretRevokes(c.Context(), unitName, []params.GrantRevokeSecretArg{{
		URI:         uri.String(),
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("two").String()},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.String(), tc.Equals, uri.String())
	c.Check(result[0].SubjectUUID, tc.Equals, "uuid-app")
	c.Check(result[0].SubjectTypeID, tc.Equals, secret.SubjectApplication)
	c.Check(result[0].OwnerKind, tc.Equals, secret.UnitCharmSecretOwner)
}

// --- resolveRevokeSubjects tests ---

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsInvalidScopeTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    "bad-tag",
		SubjectTags: []string{names.NewApplicationTag("app1").String()},
	})
	c.Assert(err, tc.ErrorMatches, `.*"bad-tag" is not a valid tag.*`)
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsInvalidSubjectTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{"not-a-tag"},
	})
	c.Assert(err, tc.ErrorMatches, `.*"not-a-tag" is not a valid tag.*`)
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsUnsupportedTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	_, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewMachineTag("0").String()},
	})
	c.Assert(err, tc.ErrorMatches, `.*tag kind "machine" not valid for secret accessor`)
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsResolveParamsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{{Error: errors.New("resolve failed")}})

	results, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String()},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].err, tc.ErrorMatches, "resolve failed")
}

func (s *UniterSecretsSuite) TestResolveRevokeSubjectsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	accessor := secret.SecretAccessor{Kind: secret.UnitAccessor, ID: "mariadb/0"}
	s.secretService.EXPECT().ResolveRevokeParams(gomock.Any(), gomock.Any()).
		Return([]secret.RevokeResult{
			{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-1", SubjectTypeID: secret.SubjectApplication}},
			{RevokeParams: secret.RevokeParams{SubjectUUID: "uuid-2", SubjectTypeID: secret.SubjectUnit}},
		})

	results, err := s.facade.resolveRevokeSubjects(c.Context(), accessor, params.GrantRevokeSecretArg{
		ScopeTag:    names.NewRelationTag("one:db two:use").String(),
		SubjectTags: []string{names.NewApplicationTag("app1").String(), names.NewUnitTag("app1/0").String()},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)
	c.Check(results[0].SubjectUUID, tc.Equals, "uuid-1")
	c.Check(results[0].SubjectTypeID, tc.Equals, secret.SubjectApplication)
	c.Check(results[0].err, tc.ErrorIsNil)
	c.Check(results[1].SubjectUUID, tc.Equals, "uuid-2")
	c.Check(results[1].SubjectTypeID, tc.Equals, secret.SubjectUnit)
	c.Check(results[1].err, tc.ErrorIsNil)
}

// --- resolveSecretOwnerKinds tests ---

func (s *UniterSecretsSuite) TestResolveSecretOwnerKindsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// No service call expected for empty input.
	result, err := s.facade.resolveSecretOwnerKinds(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func (s *UniterSecretsSuite) TestResolveSecretOwnerKindsServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), []*coresecrets.URI{uri}).
		Return(nil, errors.New("db gone"))

	_, err := s.facade.resolveSecretOwnerKinds(c.Context(), []unitstate.RevokeSecretArg{{
		URI: uri,
	}})
	c.Assert(err, tc.ErrorMatches, "db gone")
}

func (s *UniterSecretsSuite) TestResolveSecretOwnerKindsFiltersDisappeared(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	// Only return info for uri1 — uri2 "disappeared".
	s.secretService.EXPECT().GetSecretOwnerKinds(gomock.Any(), gomock.Any()).
		Return([]secret.SecretOwnerInfo{{
			SecretID:  uri1.ID,
			OwnerKind: secret.ApplicationCharmSecretOwner,
		}}, nil)

	result, err := s.facade.resolveSecretOwnerKinds(c.Context(), []unitstate.RevokeSecretArg{
		{URI: uri1, SubjectUUID: "a"},
		{URI: uri2, SubjectUUID: "b"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].URI.ID, tc.Equals, uri1.ID)
	c.Check(result[0].OwnerKind, tc.Equals, secret.ApplicationCharmSecretOwner)
}
